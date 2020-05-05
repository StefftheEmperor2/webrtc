
package main


import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc"
	"html/template"
	"io"
	"log"
	"net/http"
	"path"
	"time"
)

const (
	rtcpPLIInterval = time.Second * 3
)

type Invitation struct {
	Guest string
	Conference string
}

func newInvitation(conference string, guest string) *Invitation {
	invitation := Invitation{
		Conference: conference,
		Guest:     guest,
	}

	return &invitation
}

type Conference struct {
	Users *[]User
	Initiator *User
	Uuid *uuid.UUID
}

func (conference *Conference) addUser(user *User)  {
	users := conference.Users
	*conference.Users = append(*users, *user)
}

func newConference(initiator *User) *Conference {
	users := make([]User, 0)
	uuid, _ := uuid.NewUUID()
	return &Conference{
		Users: &users,
		Initiator: initiator,
		Uuid: &uuid,
	}
}

type ConferenceList struct {
	conferences *[]Conference
}

func (conferenceList *ConferenceList) newConference(initiator *User) *Conference {
	conference := newConference(initiator)
	conferences := conferenceList.conferences
	*conferenceList.conferences = append(*conferences, *conference)
	return conference
}

func newConferenceList() *ConferenceList {
	conferences := make([]Conference, 0)
	return &ConferenceList{conferences: &conferences}
}

type User struct {
	Username string
	Invitation chan *Invitation
	RemoteConnection *webrtc.PeerConnection
	RemoteConnectionChannel chan *string
}

func (user User) getUsername() string {
	return user.Username
}

func (user User) createRemoteConnection(offer *WebsocketOffer) string {
	fmt.Println(fmt.Sprintf("Creating remote connection for user %s", user.getUsername()))
	recvOnlyOffer := webrtc.SessionDescription{}
	DecodeOffer(offer.LocalDescription, &recvOnlyOffer)
	mediaEngine := webrtc.MediaEngine{}
	err := mediaEngine.PopulateFromSDP(recvOnlyOffer)
	if err != nil {
		panic(err)
	}

	// Create the API object with the MediaEngine
	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))

	peerConnectionConfig := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	peerConnection, err := api.NewPeerConnection(peerConnectionConfig)
	if err != nil {
		panic(err)
	}

	if _, err = peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio); err != nil {
		panic(err)
	} else if _, err = peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo); err != nil {
		panic(err)
	}

	localTrackChan := make(chan *webrtc.Track)
	// Set a handler for when a new remote track starts, this just distributes all our packets
	// to connected peers
	peerConnection.OnTrack(func(remoteTrack *webrtc.Track, receiver *webrtc.RTPReceiver) {
		// Send a PLI on an interval so that the publisher is pushing a keyframe every rtcpPLIInterval
		// This can be less wasteful by processing incoming RTCP events, then we would emit a NACK/PLI when a viewer requests it
		go func() {
			ticker := time.NewTicker(rtcpPLIInterval)
			for range ticker.C {
				if rtcpSendErr := peerConnection.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: remoteTrack.SSRC()}}); rtcpSendErr != nil {
					fmt.Println(rtcpSendErr)
				}
			}
		}()

		// Create a local track, all our SFU clients will be fed via this track
		localTrack, newTrackErr := peerConnection.NewTrack(remoteTrack.PayloadType(), remoteTrack.SSRC(), "video", "pion")
		if newTrackErr != nil {
			panic(newTrackErr)
		}
		localTrackChan <- localTrack

		rtpBuf := make([]byte, 1400)
		for {
			i, readErr := remoteTrack.Read(rtpBuf)
			if readErr != nil {
				panic(readErr)
			}

			// ErrClosedPipe means we don't have any subscribers, this is ok if no peers have connected yet
			if _, err = localTrack.Write(rtpBuf[:i]); err != nil && err != io.ErrClosedPipe {
				panic(err)
			}
		}
	})

	// Set the remote SessionDescription
	err = peerConnection.SetRemoteDescription(recvOnlyOffer)
	if err != nil {
		panic(err)
	}

	// Create answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		panic(err)
	}

	// Sets the LocalDescription, and starts our UDP listeners
	err = peerConnection.SetLocalDescription(answer)
	if err != nil {
		panic(err)
	}

	// Get the LocalDescription and take it to base64 so we can paste in browser
	user.RemoteConnection = peerConnection
	encodedAnswer := EncodeOffer(answer)
	select {
		case user.RemoteConnectionChannel <- &encodedAnswer:
		default:
	}

	return encodedAnswer
}
func newUser(Username string) *User {
	invitationChannel := make(chan *Invitation, 1)
	remoteConnectionChannel := make(chan *string, 1)
	return &User{
		Username: Username,
		Invitation: invitationChannel,
		RemoteConnectionChannel: remoteConnectionChannel,
	}
}

type UserList struct {
	users *[]User
}

func newUserList() *UserList {
	users := make([]User, 0)
	return &UserList{users: &users}
}

func (userList *UserList) has(user *User) bool {
	found := false
	for _, currentUser := range *userList.users {
		if currentUser.getUsername() == user.getUsername() {
			found = true
			break
		}
	}
	return found
}

func (userList *UserList) append(user *User)  {
	if userList.users == nil {
		newList := make([]User, 0)
		userList.users = &newList
	}
	if ! userList.has(user)	{
		*userList.users = append(*userList.users, *user)
	}
}

func (userList *UserList) remove(user *User) {
	if userList.users != nil {
		dereferencedUserList := *userList.users
		userListLength := len(dereferencedUserList)
		if userListLength > 0 {
			for key, item := range dereferencedUserList {
				if item.Username == user.Username {
					if key < len(dereferencedUserList) - 1 {
						dereferencedUserList[key] = dereferencedUserList[len(dereferencedUserList) - 1]
					}

					if userListLength > 1 {
						dereferencedUserList = dereferencedUserList[:len(dereferencedUserList)-2]
					} else {
						dereferencedUserList = make([]User, 0)
					}
				}
			}
		}
		userList.users = &dereferencedUserList
	}
}

func (userList UserList) isEmpty() bool {
	if userList.users == nil {
		return true
	} else {
		return len(*userList.users) == 0
	}
}

func (userList UserList) getFirst() *User {
	userSlice := *userList.users
	if len(userSlice) == 0 {
		return nil
	}
	user := userSlice[0]
	return &user
}

type WebsocketMessage struct {
	Object string
	Action string
	Data string
}

func (userList UserList) diffTo(anotherUserList *UserList) (*UserList, *UserList) {
	var found bool
	var added = newUserList()
	var removed = newUserList()

	if ! userList.isEmpty() {
		for _, user := range *userList.users {
			found = false
			if ! anotherUserList.isEmpty() {
				for _, anotherUser := range *anotherUserList.users {
					if user.Username == anotherUser.Username {
						found = true
						break
					}
				}
			}
			if ! found {
				added.append(&user)
			}
		}
	}

	if ! anotherUserList.isEmpty() {
		for _, anotherUser := range *anotherUserList.users {
			found = false
			if ! userList.isEmpty() {
				for _, user := range *userList.users {
					if user.Username == anotherUser.Username {
						found = true
						break
					}
				}
			}

			if ! found {
				removed.append(&anotherUser)
			}
		}
	}

	return added, removed
}

type RTCTemplate struct {
	Port int
	template *template.Template
}

type WebsocketHandler struct {
	upgrader *websocket.Upgrader
	userList *UserList
	conferenceList *ConferenceList
}

func newWebsocketHandler(upgrader *websocket.Upgrader, userList *UserList) *WebsocketHandler {
	return &WebsocketHandler{
		upgrader:       upgrader,
		userList:       userList,
		conferenceList: newConferenceList(),
	}
}
func (appInstance *AppInstance) addUser(user *User) {
	fmt.Println(fmt.Sprintf("AppInstance %p Adding User \"%s\" to handler", &appInstance, user.Username))
	*appInstance.userList.users = append(*appInstance.userList.users, *user)
}

func (appInstance *AppInstance) getMyself() *User {
	return appInstance.myself
}

func (template *RTCTemplate) handleWebRTCJS(w http.ResponseWriter, r *http.Request) {
	templateName := path.Base(r.URL.Path)
	log.Print("Serving ", templateName)
	success := template.template.Execute(w, template)

	switch success.(type) {
		case error:
			log.Print(success)
	}
}

func writeUserMessages(c *websocket.Conn, list *UserList, action string) {
	if ! list.isEmpty() {
		for _, item := range *list.users {
			message, _ := json.Marshal(WebsocketMessage{
				Object:   "User",
				Action:   action,
				Data: item.Username,
			})
			c.WriteMessage(websocket.TextMessage, message)
		}
	}
}

func writeInvitation(c *websocket.Conn, invitation *Invitation) {
	data, _ := json.Marshal(invitation)
	message, _ := json.Marshal(WebsocketMessage{
		Object:   "Call",
		Action:   "invite",
		Data: string(data),
	})
	c.WriteMessage(websocket.TextMessage, message)
}

func writeAnswer(c *websocket.Conn, answer *string) {
	fmt.Println("Writing answer to call")
	message, _ := json.Marshal(WebsocketMessage{
		Object:   "Call",
		Action:   "answer",
		Data: *answer,
	})
	c.WriteMessage(websocket.TextMessage, message)
}

func (appInstance *AppInstance) handleUserAction(websocketMessage *WebsocketMessage) {
	switch websocketMessage.Action {
		case "add":
			user := newUser(websocketMessage.Data)
			appInstance.myself = user
			appInstance.addUser(user)
			break
	}
}

func (appInstance *AppInstance) isLoggedIn() bool {
	return ! (appInstance.myself == nil)
}

type WebsocketOffer struct {
	Users []string
	LocalDescription string
}

func DecodeOffer(in string, obj interface{}) {
	b, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		panic(err)
	}

	err = json.Unmarshal(b, obj)
	if err != nil {
		panic(err)
	}
}

func EncodeOffer(obj interface{}) string {
	b, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}

	return base64.StdEncoding.EncodeToString(b)
}

func (appInstance *AppInstance) createConference(initiator *User) *Conference {
	return appInstance.handler.conferenceList.newConference(initiator)
}
func (appInstance *AppInstance) handleCallAction(
	connection *websocket.Conn,
	websocketMessage *WebsocketMessage,
	userList *UserList) {
	switch websocketMessage.Action {
		case "offer":
			websocketOffer := &WebsocketOffer{}
			myself := appInstance.getMyself()
			ownUserName := myself.getUsername()

			json.Unmarshal([]byte(websocketMessage.Data), websocketOffer)
			myself.createRemoteConnection(websocketOffer)
			conference := appInstance.createConference(myself)
			conference.addUser(myself)

			for _, offerUserName := range websocketOffer.Users {
				for _, user := range *userList.users {
					if user.Username == offerUserName {
						conference.addUser(&user)
						fmt.Println(fmt.Sprintf("Inviting %s", user.getUsername()))
						select {
							case user.Invitation <- newInvitation(conference.Uuid.String(), ownUserName):
							default:
								fmt.Println(fmt.Sprintf("Invitation Channel of %s is already full", user.getUsername()))
						}
					}
				}
			}

			break
		case "accepted":
			myself := appInstance.getMyself()
			websocketOffer := &WebsocketOffer{}
			json.Unmarshal([]byte(websocketMessage.Data), websocketOffer)
			myself.createRemoteConnection(websocketOffer)
	}
}

func (appInstance *AppInstance) readMessages() {
	for {
		websocketMessage := WebsocketMessage{}
		err := appInstance.websocketConnection.ReadJSON(&websocketMessage)
		if err != nil {
			log.Println("read:", err)
			break
		}

		switch websocketMessage.Object {
			case "User":
				appInstance.handleUserAction(&websocketMessage)
				break
			case "Call":
				appInstance.handleCallAction(appInstance.websocketConnection, &websocketMessage, appInstance.userList)
				break
		}
	}

}

type AppInstance struct {
	userList *UserList
	websocketConnection *websocket.Conn
	myself *User
	handler WebsocketHandler
}

func (appInstance *AppInstance) run(userList *UserList) {
	var inAppUserlistOnly *UserList
	var inGlobalUserlistOnly *UserList
	isInitialized := false
	go appInstance.readMessages()
	for {
		if ! appInstance.isLoggedIn() {
			continue
		}

		if  ! isInitialized {
			for _, currentUser := range *userList.users {
				initUserList := newUserList()
				if &currentUser != appInstance.myself {
					appInstance.userList.append(&currentUser)
					initUserList.append(&currentUser)
				}
				writeUserMessages(appInstance.websocketConnection, initUserList, "add")
				initUserList = nil
			}
			isInitialized = true
		}
		inAppUserlistOnly, inGlobalUserlistOnly = appInstance.userList.diffTo(userList)

		writeUserMessages(appInstance.websocketConnection, inAppUserlistOnly, "add")

		writeUserMessages(appInstance.websocketConnection, inGlobalUserlistOnly, "add")

		for _, user := range *inAppUserlistOnly.users {
			userList.append(&user)
		}

		for _, user := range *inGlobalUserlistOnly.users {
			appInstance.userList.append(&user)
		}

		if ! appInstance.userList.isEmpty() {
			select {
			case invitation := <-appInstance.getMyself().Invitation:
				fmt.Println(fmt.Sprintf("Sending invitation to %s", appInstance.getMyself().getUsername()))
				writeInvitation(appInstance.websocketConnection, invitation)
			case answer := <-appInstance.getMyself().RemoteConnectionChannel:
				writeAnswer(appInstance.websocketConnection, answer)
			default:

			}
		}

	}
}
func (handler WebsocketHandler) handleWebsocket(w http.ResponseWriter, r *http.Request) {
	c, err := handler.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()

	userList := newUserList()
	appInstance := AppInstance{
		userList: userList,
		websocketConnection: c,
		handler: handler,
	}
	appInstance.run(handler.userList)
}

func createTemplate(name string) *template.Template {
	templateName := path.Base(name)
	tmplInstance := template.New(templateName)
	template.Must(tmplInstance.ParseFiles(fmt.Sprintf("./%s", name)))

	return tmplInstance
}

func main() {
	upgrader := websocket.Upgrader{}
	port := 8080
	rtcTemplate := &RTCTemplate{
		port,
		createTemplate("/templates/js/webRTC.js"),
	}
	userList := newUserList()
	websocketHandler := newWebsocketHandler(&upgrader, userList)
	http.Handle("/", http.FileServer(http.Dir("./static")))
	http.HandleFunc("/websocket", websocketHandler.handleWebsocket)
	http.HandleFunc("/templates/js/webRTC.js", rtcTemplate.handleWebRTCJS)

	err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	if err != nil {
		log.Fatal(err)
	}
}