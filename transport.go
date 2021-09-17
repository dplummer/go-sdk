package statsig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
)

const (
	maxRetries        = 5
	backoffMultiplier = 10
)

type statsigMetadata struct {
	SDKType    string `json:"sdkType"`
	SDKVersion string `json:"sdkVersion"`
}

type transport struct {
	api      string
	sdkKey   string
	metadata statsigMetadata
	client   *http.Client
}

func newTransport(secret string, api string, sdkType string, sdkVersion string) *transport {
	api = defaultString(api, DefaultEndpoint)
	api = strings.TrimSuffix(api, "/")
	sdkType = defaultString(sdkType, "go-sdk")
	sdkVersion = defaultString(sdkVersion, "0.4.2")

	return &transport{
		api:      api,
		metadata: statsigMetadata{SDKType: sdkType, SDKVersion: sdkVersion},
		sdkKey:   secret,
		client:   &http.Client{},
	}
}

func (n *transport) postRequest(
	endpoint string,
	in interface{},
	out interface{},
) error {
	return n.postRequestInternal(endpoint, in, out, 0, 0)
}

func (n *transport) retryablePostRequest(
	endpoint string,
	in interface{},
	out interface{},
	retries int,
) error {
	return n.postRequestInternal(endpoint, in, out, retries, time.Second)
}

func (n *transport) postRequestInternal(
	endpoint string,
	in interface{},
	out interface{},
	retries int,
	backoff time.Duration,
) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}

	return retry(retries, time.Duration(backoff), func() (bool, error) {
		req, err := http.NewRequest("POST", path.Join(n.api, endpoint), bytes.NewBuffer(body))
		if err != nil {
			return false, err
		}

		req.Header.Add("STATSIG-API-KEY", n.sdkKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Add("STATSIG-CLIENT-TIME", strconv.FormatInt(time.Now().Unix()*1000, 10))

		response, err := n.client.Do(req)
		if err != nil {
			return true, err
		}
		defer response.Body.Close()

		if response.StatusCode >= 200 && response.StatusCode < 300 {
			return false, json.NewDecoder(response.Body).Decode(&out)
		}

		return shouldRetry(response.StatusCode), fmt.Errorf("http response error code: %d", response.StatusCode)
	})
}

func retry(retries int, backoff time.Duration, fn func() (bool, error)) error {
	for {
		if retry, err := fn(); retry {
			if retries <= 0 {
				return err
			}

			retries--
			time.Sleep(backoff)
			backoff = backoff * backoffMultiplier
		} else {
			return err
		}
	}
}

func shouldRetry(code int) bool {
	switch code {
	case 408, 500, 502, 503, 504, 522, 524, 599:
		return true
	default:
		return false
	}
}
