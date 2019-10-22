// IP regular expression
const REGEX = /^(([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])\.){3}([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5]):[0-9]+$/g

$(document).ready(function() {

    // Perfect scrollbars
    const ps1 = new PerfectScrollbar('.messageBox');
    const ps2 = new PerfectScrollbar('.list-group');

    var modal = document.getElementById("privateMsgModal");
    var clse = document.getElementsByClassName("close")[0];

    clse.onclick = function() {
        modal.style.display = "none";
    }

    window.onclick = function(event) {
        if (event.target == modal) {
            modal.style.display = "none";
        }
    }

    $(".users").on("dblclick", ".list-group-item", function(e) {
        name = $(e.target).text()
        
        modal.style.display = "block";
        $("#peerName").text(name)
    })

    $("#fileForm").submit(function(e) {
        e.preventDefault();

        var fileInput = document.getElementById('file');
        var file = fileInput.files[0];
        var formData = new FormData();
        formData.append(file.name, file);

        var xhr = new XMLHttpRequest();

        xhr.onreadystatechange = function() {
            if (xhr.readyState === 4 && xhr.status == 200) {
                document.getElementById("file").value = null;
            }
        }

        xhr.open('POST', '/file', true);
        xhr.send(formData)
    })

    // Private message submit
    $("#privateMsgForm").submit(function(e) {

        // Prevent default, get value, clean value/errors, check is not empty and send ajax
        e.preventDefault();

        var actionurl = e.currentTarget.action;
        var input = $("#privateMsgForm #privateMsgTextArea").val();
        $("#privateMsgForm #privateMsgTextArea").val("");

        $("#msgForm .toast").each(function() {
            $(this).remove();
        });

        if(input != "") {
            var msg = {
                Text: input,
                Destination: $("#peerName").text()
            };

            $.ajax({
                    url: actionurl + 'users',
                    type: 'post',
                    data: JSON.stringify(msg),
                    dataType: 'json',
                    contentType: 'application/json',
                    success: function(data, textStatus, request) {
                        if(request.status == 200) {
                            console.log("Ok message" + jsn)
                        }
                    }
            });

        } else {
            $("<div class=\"toast\" role=\"alert\" aria-live=\"assertive\" aria-atomic=\"true\"><div class=\"toast-header\"><strong class=\"mr-auto\">Error Message</strong><button type=\"button\" class=\"ml-2 mb-1 close\" data-dismiss=\"toast\" aria-label=\"Close\"><span aria-hidden=\"true\">&times;</span></button></div><div class=\"toast-body\">The message must be non empty</div></div>").appendTo("#peersterForm");
        }

    });

    // Message submit
    $("#msgForm").submit(function(e) {

        // Prevent default, get value, clean value/errors, check is not empty and send ajax
        e.preventDefault();

        var actionurl = e.currentTarget.action;
        var input = $("#msgForm #msgTextArea").val();
        $("#msgForm #msgTextArea").val("");

        $("#msgForm .toast").each(function() {
            $(this).remove();
        });

        if(input != "") {
            var jsn = input;

            $.ajax({
                    url: actionurl + 'messages',
                    type: 'post',
                    data: jsn,
                    success: function(data, textStatus, request) {
                        if(request.status == 200) {
                            console.log("Ok message" + jsn)
                        }
                    }
            });

        } else {
            $("<div class=\"toast\" role=\"alert\" aria-live=\"assertive\" aria-atomic=\"true\"><div class=\"toast-header\"><strong class=\"mr-auto\">Error Message</strong><button type=\"button\" class=\"ml-2 mb-1 close\" data-dismiss=\"toast\" aria-label=\"Close\"><span aria-hidden=\"true\">&times;</span></button></div><div class=\"toast-body\">The message must be non empty</div></div>").appendTo("#peersterForm");
        }

    });

    $("#peersterForm").submit(function(e) {

         // Prevent default, get value, clean value/errors, check regex test is true and send ajax
        e.preventDefault();

        var actionurl = e.currentTarget.action;
        var input = $("#peersterForm #peersterInput").val();
        $("#peersterForm #peersterInput").val("")

        $("#peersterForm .toast").each(function() {
            $(this).remove();
        });

        if(REGEX.test(input)) {
            var jsn = input;

            $.ajax({
                    url: actionurl + 'nodes',
                    type: 'post',
                    data: jsn,
                    success: function(data, textStatus, request) {
                        if(request.status == 200) {
                            console.log("Ok peerster" + jsn)
                        }
                    }
            });

        } else {
            $("<div class=\"toast\" role=\"alert\" aria-live=\"assertive\" aria-atomic=\"true\"><div class=\"toast-header\"><strong class=\"mr-auto\">Error Peerster Address</strong><button type=\"button\" class=\"ml-2 mb-1 close\" data-dismiss=\"toast\" aria-label=\"Close\"><span aria-hidden=\"true\">&times;</span></button></div><div class=\"toast-body\">Invalid peerster address, the address must follow the pattern 127.127.127.1:9090</div></div>").appendTo("#peersterForm");
        }   

    });

    // Get the node ID
    $.ajax({
        url: window.location.href  + "id",
        type: 'get',
        success: function(data) {
            $("#peerID").text(data.ID)
        }
    });

    // Get peerster addresses and messages
    getMessages();
    getNodes();
    getUsers()

    // Get new peersters and messages each two seconds
    setInterval(() => {
        getNodes();
        getMessages();
        getUsers();
    },  2000);
    
})

// Get the peersters addresses and if its new add it to the list
function getNodes() {
    $.ajax({
        url: window.location.href  + "nodes",
        type: 'get',
        success: function(data) {
            let peers = data.Peers.split(",");
            peers.forEach(function(peer) {
                if($(".list-group.peers .list-group-item").text().indexOf(peer) == -1) {
                    $("<li class=\"list-group-item\">"+peer+"</li>").prependTo(".list-group.peers")
                    $(".list-group.peers").scrollTop($(".list-group.peers")[0].scrollHeight);
                }
            });
        }
    });
}

function getUsers() {
    $.ajax({
        url: window.location.href  + "users",
        type: 'get',
        success: function(data) {
            let peers = data.Peers.split(",");
            peers.forEach(function(peer) {
                if($(".list-group.users .list-group-item").text().indexOf(peer) == -1) {
                    $("<li class=\"list-group-item\">"+peer+"</li>").prependTo(".list-group.users")
                    $(".list-group.users").scrollTop($(".list-group.users")[0].scrollHeight);
                }
            });
        }
    });
}

// Get the messages and if its new add it to the list
function getMessages() {
    $.ajax({
        url: window.location.href  + "messages",
        type: 'get',
        success: function(data) {
            var keys = [];
            let rumors = data.Rumors;

            for(let k in data.Rumors) {
                keys.push(k);
            }

            keys.forEach(function(node) {
                let origins = {
                    ids: [],
                    nodes: []
                }
                
                $(".message").each(function() {
                    origins.ids.push($(this).find('.card-subtitle').text())
                    origins.nodes.push($(this).find('.card-title').text())
                })
                // If node is not new add only the new messages
                if(origins.nodes.indexOf(node) >= 0) {

                    var msgs = {
                        ids: [],
                        nodes: []
                    };

                    origins.nodes.forEach(function(val, ind) {
                        if(val.indexOf(node) !== -1) {
                            msgs.ids.push(origins.ids[ind]);
                            msgs.nodes.push(origins.nodes[ind]);
                        }
                    });

                    let lastID = 0;
                    rumors[node].forEach(function(rumor) {
                        if(rumor.Text == "")
                            return

                        if(rumor.ID > msgs.ids[msgs.ids.length -1] && rumor.ID > lastID) {
                            lastID = rumor.ID;
                            
                            let newmsg = $("<div class=\"card message\"><div class=\"card-body\"><h5 class=\"card-title\">"+ rumor.Origin +"</h5><h5 class=\"card-subtitle mb-2 text-muted\">"+ rumor.ID +"</h5><p class=\"card-text\">" + rumor.Text + "</p></div></div>").appendTo(".messageBox")
                            if(node == $("#peerID").text())
                                newmsg.addClass("mine")

                            $(".messageBox").scrollTop($(".messageBox")[0].scrollHeight);
                        }                    
                    })
                // If node is new add all messages
                } else {
                    rumors[node].forEach(function(rumor) {
                        if(rumor.Text == "")
                            return
            
                        let newmsg = $("<div class=\"card message\"><div class=\"card-body\"><h5 class=\"card-title\">"+ rumor.Origin +"</h5><h5 class=\"card-subtitle mb-2 text-muted\">"+ rumor.ID +"</h5><p class=\"card-text\">" + rumor.Text + "</p></div></div>").appendTo(".messageBox")
                        // Check if message is originaly from me
                        if(node == $("#peerID").text())
                            newmsg.addClass("mine")

                        $(".messageBox").scrollTop($(".messageBox")[0].scrollHeight);
                    })
                }
            });
        }
    });
}