
# goradius
A simple implementation of a RADIUS server in go. The policy flow is modeled HTTP-like, through the use of middleware in a request/response flow. Look at example below

### TODO
* Handle vendor specific attributes
* Handle passwords > 16 chars

### Example

```go
package main

import (
    "bytes"
    "github.com/rem7/goradius"
    "log"
)

// echo "User-Name=steve,User-Password=testing" | radclient -sx 127.0.0.1:1812 auth secret

func main() {

    log.Printf("Server started")
    server := goradius.RadiusServer{}

    // server.Handler(passwordCheck)

    // Add middleware
    server.Use(passwordCheck)
    server.Use(addAttributes)
    server.ListenAndServe("0.0.0.0:1812", "s3cr37")

}

func passwordCheck(req, res *goradius.RadiusPacket) error {

    username := req.GetAttribute("User-Name")
    usernameData, _ := username.([]byte)

    password := req.GetAttribute("User-Password")
    passwordData, _ := password.([]byte)

    if bytes.Equal(passwordData, []byte("testing")) &&
        bytes.Equal(usernameData, []byte("steve")) {
        res.Code = goradius.AccessAccept
    } else {
        res.Code = goradius.AccessReject
    }

    return nil

}

func addAttributes(req, res *goradius.RadiusPacket) error {

    if res.Code == goradius.AccessAccept {
        res.AddAttribute("NAS-Identifier", []byte("rem7"))
        res.AddAttribute("Idle-Timeout", uint32(600))
        res.AddAttribute("Session-Timeout", uint32(10800))
    }

    return nil
}

```