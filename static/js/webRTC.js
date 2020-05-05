window.addEventListener("load", function() {
    let cUserList = function()
    {
        let users = [], selection = null, myself = null;

        this.add = function (user) {
            users.push(user)
        };

        this.remove = function (user) {
            let elem;
            for (let i = 0; i<this.getLength();)
            {
                elem = users[i]
                if (elem.getUserName() === user.getUserName())
                {
                    users.splice(i, 1);
                }
                else
                {
                    i++;
                }
            }
        };

        this.getLength = function()
        {
            return users.length;
        };

        this.getUserAt = function(index)
        {
            return users[index];
        };

        this.getUserByUserName = function(userName)
        {
            let user = null, currentUser;
            for (let i = 0; i<this.getLength();i++)
            {
                currentUser = this.getUserAt(i);
                if (currentUser.getUserName() === userName)
                {
                    user = currentUser;
                    break;
                }
            }
            return user;
        };

        this.getSelection = function () {
            if (selection === null)
            {
                selection = new cUserList();
            }

            return selection;
        };

        this.isUserSelected = function(user) {
            let selection = this.getSelection(), isSelected = false;
            for (let i = 0; i<selection.getLength();i++)
            {
                if (selection.getUserAt(i).getUserName() === user.getUserName())
                {
                    isSelected = true;
                    break;
                }
            }

            return isSelected;
        };

        this.toggleUserSelected = function (user)
        {
            if (this.isUserSelected(user))
            {
                user.getDomElement().classList.remove('selected');
                this.getSelection().remove(user);
            }
            else
            {
                user.getDomElement().classList.add('selected');
                this.getSelection().add(user);
            }
        };

        this.setMyself = function(paramMyself)
        {
            myself = paramMyself;
        }

        this.getSelectedUsers = function(withMyself)
        {
            if (typeof withMyself == 'undefined')
            {
                withMyself = true;
            }

            let users = [], selection = this.getSelection(), currentUser;
            for (let i=0; i<selection.getLength();i++)
            {
                currentUser = selection.getUserAt(i);
                if ((!withMyself && currentUser !== myself) || withMyself)
                {
                    users.push(currentUser);
                }
            }

            return users
        }

        this.getSelectedUserNames = function(withMyself)
        {
            let users = this.getSelectedUsers(withMyself), userNames = []

            for (let i=0; i<users.length;i++)
            {
                userNames.push(users[i].getUserName());
            }

            return userNames;
        }
    };

    let cUser = function(userName)
    {
        let domElement;

        domElement = document.createElement('LI');
        domElement.innerText = userName;

        this.getUserName = function()
        {
            return userName;
        }

        this.getDomElement = function () {
            return domElement;
        };
    };

    let cVideo = function()
    {
        let domElement = document.createElement('video');

        this.getDomElement = function()
        {
            return domElement;
        };

        this.setSrcObject = function (paramSrcObject) {
            this.getDomElement().srcObject = paramSrcObject;
        };
    }

    let cVideoList = function ()
    {
        let videos = [];

        this.createVideo = function()
        {
            let video = new cVideo();
            videos.push(video);

            return video;
        };

        this.getVideos = function () {
            return videos;
        };
    };

    let registerLogin = function () {
        let loginButton = document.getElementById('login_button'),
            usernameField = document.getElementById('username'),
            userListDomElement = document.getElementById('user_list'),
            videoListDomElement = document.getElementById('video_list'),
            userList = new cUserList(),
            ws,
            myself,
            videoList = new cVideoList(),
            peerConnection = null,
            app = this;

        this.getPeerConnection = function()
        {
            if (peerConnection === null) {
                peerConnection = new RTCPeerConnection({
                    iceServers: [
                        {
                            urls: 'stun:stun.l.google.com:19302'
                        }
                    ]
                });

                navigator.mediaDevices.getUserMedia({ video: true, audio: true })
                    .then(stream => {
                        stream.getTracks().forEach(function(track) {
                            peerConnection.addTrack(track, stream);
                        });
                        let video = videoList.createVideo()
                        video.setSrcObject(stream);
                        videoListDomElement.appendChild(video.getDomElement())
                        peerConnection.createOffer().then(d => {
                            console.log('Offer created');
                            peerConnection.setLocalDescription(d);
                        }).catch(console.log);
                    }).catch(console.log);
            }

            peerConnection.oniceconnectionstatechange = () => console.log('ICE Connection state changed ', peerConnection.iceConnectionState)

            return peerConnection;
        }

        let dispatchUserEvent = function (eventData)
        {
            let user;
            switch (eventData.Action)
            {
                case "add":
                    let isMyself = false;
                    if (eventData.Data === myself.getUserName())
                    {
                        user = myself;
                        isMyself = true;
                    }
                    else
                    {
                        user = new cUser(eventData.Data);
                    }

                    let userDomElement = user.getDomElement();
                    userListDomElement.appendChild(userDomElement);
                    if (isMyself)
                    {
                        userList.toggleUserSelected(user);
                        userDomElement.classList.add('myself')
                    }
                    else
                    {
                        userDomElement.addEventListener('click', function () {
                            userList.toggleUserSelected(user);
                        });
                    }

                    userList.add(user);
                    break;
                case "remove":
                    user = userList.getUserByUserName(eventData.Data);
                    if (userList.isUserSelected(user))
                    {
                        userList.toggleUserSelected(user);
                    }
                    userListDomElement.removeChild(user.getDomElement());
                    user.getDomElement().remove();
                    break;
            }
        };

        let dispatchCallEvent = function(eventData)
        {
            let peerConnection;
            switch (eventData.Action) {
                case "invite":
                        peerConnection = app.getPeerConnection();
                        document.body.className = 'active_call';
                        peerConnection.onicecandidate = event => {
                            if (event.candidate === null) {
                                let localDescription = btoa(JSON.stringify(peerConnection.localDescription));
                                ws.send(JSON.stringify({
                                    "Object": 'Call',
                                    "Action": "accepted",
                                    "Data": JSON.stringify({
                                        "LocalDescription": localDescription
                                    })
                                }));
                            }
                        }
                    break;
                case "answer":
                    peerConnection = app.getPeerConnection();
                    peerConnection.setRemoteDescription(new RTCSessionDescription(JSON.parse(atob(eventData.Data)))).then(() => console.log('remote description set'))
            }
        }

        loginButton.addEventListener('click', function () {
            ws = new WebSocket("ws://" + window.location.hostname + ':' + window.websocketPort + "/websocket");
            ws.onopen = function () {
                console.log("OPEN");
                document.body.className = 'user_list';
                myself = new cUser(usernameField.value);
                document.title = 'WebRTC Test :: ' + myself.getUserName();
                userList.setMyself(myself);
                ws.send(JSON.stringify({"Object": "User", "Action": "add", "Data": usernameField.value}));
            }

            ws.onclose = function () {
                console.log("CLOSE");
                ws = null;
            }

            ws.onmessage = function (evt) {
                let eventData = JSON.parse(evt.data);
                switch (eventData.Object) {
                    case "User":
                        dispatchUserEvent(eventData);
                        break;
                    case "Call":
                        dispatchCallEvent(eventData);
                }
                console.log("RESPONSE: " + evt.data);
            }

            ws.onerror = function (evt) {
                console.log("ERROR: " + evt.data);
            }

            document.getElementById('call_button').addEventListener('click', function () {
                let peerConnection = app.getPeerConnection(),
                    localDescription;

                document.body.className = 'active_call';

                peerConnection.onicecandidate = event => {
                    if (event.candidate === null) {
                        localDescription = btoa(JSON.stringify(peerConnection.localDescription));
                        ws.send(JSON.stringify({
                            "Object": 'Call',
                            "Action": "offer",
                            "Data": JSON.stringify({
                                    "Users": userList.getSelectedUserNames(false),
                                    "LocalDescription": localDescription
                                })
                        }));
                    }
                }
            });
        });
    };

    registerLogin();
});