package simulate

import (
	"strings"

	"github.com/disaster37/gobot-simulate/server"
)

// ARESTSerialResponse handles one line of the aREST serial protocol and
// returns the JSON response line. The serial gobot-arest client writes paths
// such as "/digital/3/1" and expects a newline-terminated JSON object.
func (s *Simulator) ARESTSerialResponse(boardID, line string) string {
	line = strings.TrimSpace(strings.Trim(line, "\r\n"))
	params := ""
	if index := strings.Index(line, "?"); index >= 0 {
		query := line[index+1:]
		line = line[:index]
		for _, part := range strings.Split(query, "&") {
			key, value, ok := strings.Cut(part, "=")
			if ok && key == "params" {
				params = value
			}
		}
	}
	if !strings.HasPrefix(line, "/") {
		line = "/" + line
	}

	method := "GET"
	segments := strings.Split(strings.TrimPrefix(line, "/"), "/")
	switch {
	case len(segments) == 3 && segments[0] == "mode":
		method = "POST"
	case len(segments) == 3 && segments[0] == "digital":
		method = "POST"
	case params != "":
		method = "POST"
	}

	status, body := server.ARESTResponse(s, boardID, method, line, params)
	if status >= 400 && len(body) == 0 {
		return `{"error":{"message":"request failed"}}` + "\n"
	}
	return string(body) + "\n"
}
