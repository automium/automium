package monitoring

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func defaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}

func prepareHTTPRequest(method, endpoint string, body io.Reader) (*http.Request, error) {

	// Prepare the basic request
	baseReq, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return nil, err
	}

	// Add the authorization
	baseReq.Header.Add("Authorization", fmt.Sprintf("Bearer %s", os.Getenv("RANCHER_CLUSTER_TOKEN")))

	return baseReq, nil
}
