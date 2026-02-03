package push

import "net/http"

var sharedHTTPClient = http.DefaultClient

func SetSharedHTTPClient(c *http.Client) {
	if c != nil {
		sharedHTTPClient = c
	}
}

