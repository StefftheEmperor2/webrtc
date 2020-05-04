
package main


import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"html/template"
	"log"
	"net/http"
	"path"
)

type User struct {
	Username string
}

type UserList struct {
	users *[]User
}

func (userList *UserList) append(user *User)  {
	if userList.users == nil {
		newList := make([]User, 0)
		userList.users = &newList
	}
	*userList.users = append(*userList.users, *user)
}

func (userList UserList) isEmpty() bool {
	if userList.users == nil {
		return true
	} else {
		return len(*userList.users) == 0
	}
}

type WebsocketMessage struct {
	Object string
	Action string
	UserName string
}

func (userList UserList) diffTo(anotherUserList *UserList) (UserList, UserList) {
	var found bool
	var added UserList
	var removed UserList

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
}

func (handler WebsocketHandler) addUser(user *User) {
	if handler.userList.users == nil {
		userList := make([]User, 0)
		handler.userList.users = &userList
	}
	*handler.userList.users = append(*handler.userList.users, *user)
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

func writeUserMessages(c *websocket.Conn, list UserList, action string) {
	if ! list.isEmpty() {
		for _, item := range *list.users {
			message, _ := json.Marshal(WebsocketMessage{
				Object:   "User",
				Action:   action,
				UserName: item.Username,
			})
			c.WriteMessage(websocket.TextMessage, message)
		}
	}
}

func (handler WebsocketHandler) handleUserAction(connection *websocket.Conn, websocketMessage *WebsocketMessage) {
	switch websocketMessage.Action {
		case "add":
			handler.addUser(&User{Username: websocketMessage.UserName})
	}
}

func (handler WebsocketHandler) readMessages(c *websocket.Conn) {
	for {
		websocketMessage := WebsocketMessage{}
		err := c.ReadJSON(&websocketMessage)
		if err != nil {
			log.Println("read:", err)
			break
		}

		switch websocketMessage.Object {
			case "User":
				handler.handleUserAction(c, &websocketMessage)
				break
		}
	}

}
func (handler WebsocketHandler) handleWebsocket(w http.ResponseWriter, r *http.Request) {
	var added UserList
	var removed UserList
	c, err := handler.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()

	userList := &UserList{}

	go handler.readMessages(c)
	for {

		added, removed = handler.userList.diffTo(userList)

		writeUserMessages(c, added, "add")
		writeUserMessages(c, removed, "remove")

	}
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
	userList := &UserList{}
	websocketHandler := &WebsocketHandler{&upgrader, userList}
	http.Handle("/", http.FileServer(http.Dir("./static")))
	http.HandleFunc("/websocket", websocketHandler.handleWebsocket)
	http.HandleFunc("/templates/js/webRTC.js", rtcTemplate.handleWebRTCJS)

	err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	if err != nil {
		log.Fatal(err)
	}
}