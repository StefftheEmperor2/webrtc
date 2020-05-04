window.addEventListener("load", function(evt) {
    var register_login = function()
    {
        var login_button = document.getElementById('login_button'),
            username_field = document.getElementById('username'),
            ws, username;

        login_button.addEventListener('click', function (ev) {
            ws = new WebSocket("ws://"+window.location.hostname+':'+window.websocketPort+"/websocket");
            ws.onopen = function(evt) {
                console.log("OPEN");
                document.body.className = 'user_list';

                ws.send(JSON.stringify({"Object": "User", "Action": "add", "UserName": username_field.value}));
            }

            ws.onclose = function(evt) {
                console.log("CLOSE");
                ws = null;
            }

            ws.onmessage = function(evt) {
                console.log("RESPONSE: " + evt.data);
            }

            ws.onerror = function(evt) {
                console.log("ERROR: " + evt.data);
            }
        });
    }

    register_login();
});