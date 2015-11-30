package ovh

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mitchellh/go-homedir"
	"gopkg.in/ini.v1"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"
)

// TIMEOUT api requests after 180s
const TIMEOUT = 180

// Endpoint reprensents an API endpoint
type Endpoint string

// ENDPOINTS conveniently maps endpoints names to their real URI
var ENDPOINTS = map[string]Endpoint{
	"ovh-eu":        Endpoint("https://eu.api.ovh.com/1.0"),
	"ovh-ca":        Endpoint("https://ca.api.ovh.com/1.0"),
	"kimsufi-eu":    Endpoint("https://eu.api.kimsufi.com/1.0"),
	"kimsufi-ca":    Endpoint("https://ca.api.kimsufi.com/1.0"),
	"soyoustart-eu": Endpoint("https://eu.api.soyoustart.com/1.0"),
	"soyoustart-ca": Endpoint("https://ca.api.soyoustart.com/1.0"),
	"runabove-ca":   Endpoint("https://api.runabove.com/1.0"),
}

// Client represents an an OVH API client
type Client struct {
	endpoint          Endpoint
	applicationKey    string
	applicationSecret string
	consumerKey       string
	Timeout           time.Duration
	timeDelta         int64
	client            *http.Client
}

// APIResponse represents a response from OVH API
type APIResponse struct {
	StatusCode int
	Status     string
	Body       []byte
}

// APIError represents an unmarshalled reponse from OVH in case of error
type APIError struct {
	ErrorCode string `json:"errorCode"`
	HTTPCode  string `json:"httpCode"`
	Message   string `json:"message"`
}

// NewDefaultClient returns an OVH API Client from external configuration
func NewDefaultClient() (*Client, error) {
	return NewClient("", "", "", "")
}

// NewEndpointClient returns an OVH API Client from external configuration, for a specific endpoint
func NewEndpointClient(endpoint string) (*Client, error) {
	return NewClient(endpoint, "", "", "")
}

// NewClient returns an OVH API Client.
func NewClient(endpointName, applicationKey, applicationSecret, consumerKey string) (*Client, error) {
	// Load configuration files. Only load file from user home if home could be resolve
	cfg, err := ini.Load("/etc/ovh.conf")
	if home, err := homedir.Dir(); err == nil {
		cfg.Append(home + "/.ovh.conf")
	}
	cfg.Append("./ovh.conf")

	// Canonicalize configuration
	if endpointName == "" {
		endpointName = getConfigValue(cfg, "default", "endpoint")
	}

	if applicationKey == "" {
		applicationKey = getConfigValue(cfg, endpointName, "application_key")
	}

	if applicationSecret == "" {
		applicationSecret = getConfigValue(cfg, endpointName, "application_secret")
	}

	if consumerKey == "" {
		consumerKey = getConfigValue(cfg, endpointName, "consumer_key")
	}

	var endpoint Endpoint
	if strings.Contains(endpointName, "/") {
		endpoint = Endpoint(endpointName)
	} else {
		endpoint = ENDPOINTS[endpointName]
	}

	// Timeout
	timeout := time.Duration(TIMEOUT * time.Second)

	// Create client
	client := &Client{endpoint, applicationKey, applicationSecret, consumerKey, timeout, 0, &http.Client{}}

	// Account for clock delay with API in signatures
	timeDelta, err := client.GetUnAuth("/auth/time")
	if err != nil {
		return nil, err
	}

	var serverTime int64
	err = json.Unmarshal(timeDelta.Body, &serverTime)
	if err != nil {
		return nil, err
	}
	client.timeDelta = time.Now().Unix() - serverTime

	return client, nil
}

// getConfigValue returns the value of OVH_<NAME> or ``name`` value from ``section``
func getConfigValue(cfg *ini.File, section, name string) string {
	// Attempt to load from environment
	fromEnv := os.Getenv("OVH_" + strings.ToUpper(name))
	if len(fromEnv) > 0 {
		return fromEnv
	}

	// Attempt to load from configuration
	fromSection := cfg.Section(section)
	if fromSection == nil {
		return ""
	}

	fromSectionKey := fromSection.Key(name)
	if fromSectionKey == nil {
		return ""
	}
	return fromSectionKey.String()
}

//
// High level API
//

// DecodeError return error on unexpected HTTP code
func (r *APIResponse) DecodeError(expectedHTTPCode []int) (*APIError, error) {
	for _, code := range expectedHTTPCode {
		if r.StatusCode == code {
			return nil, nil
		}
	}

	// Decode OVH error informations from response
	if r.Body != nil {
		var ovhResponse *APIError
		err := json.Unmarshal(r.Body, ovhResponse)
		if err == nil {
			return ovhResponse, errors.New(ovhResponse.Message)
		}
	}
	return nil, fmt.Errorf("%d - %s", r.StatusCode, r.Status)
}

// Get Issues an authenticated get request on /path
func (c *Client) Get(path string) (*APIResponse, error) {
	return c.Call("GET", path, nil, true)
}

// GetUnAuth Issues an un-authenticated get request on /path
func (c *Client) GetUnAuth(path string) (*APIResponse, error) {
	return c.Call("GET", path, nil, false)
}

// Post Issues an authenticated get request on /path
func (c *Client) Post(path string, data interface{}) (*APIResponse, error) {
	return c.Call("POST", path, data, true)
}

// PostUnAuth Issues an un-authenticated get request on /path
func (c *Client) PostUnAuth(path string, data interface{}) (*APIResponse, error) {
	return c.Call("POST", path, data, false)
}

// Put Issues an authenticated get request on /path
func (c *Client) Put(path string, data interface{}) (*APIResponse, error) {
	return c.Call("PUT", path, data, true)
}

// PutUnAuth Issues an un-authenticated get request on /path
func (c *Client) PutUnAuth(path string, data interface{}) (*APIResponse, error) {
	return c.Call("PUT", path, data, false)
}

// Delete Issues an authenticated get request on /path
func (c *Client) Delete(path string) (*APIResponse, error) {
	return c.Call("DELETE", path, nil, true)
}

// DeleteUnAuth Issues an un-authenticated get request on /path
func (c *Client) DeleteUnAuth(path string) (*APIResponse, error) {
	return c.Call("DELETE", path, nil, false)
}

//
// Low Level Helpers
//

// Call calls OVH's API and signs the request if ``needAuth`` is ``true``
func (c *Client) Call(method, path string, data interface{}, needAuth bool) (*APIResponse, error) {
	target := fmt.Sprintf("%s%s", c.endpoint, path)
	timestamp := time.Now().Unix() - c.timeDelta

	var body []byte
	var err error

	if data != nil {
		body, err = json.Marshal(data)
		if err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequest(method, target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	if body != nil {
		req.Header.Add("Content-Type", "application/json;charset=utf-8")
	}
	req.Header.Add("X-Ovh-Application", c.applicationKey)

	// Some methods do not need authentication, especially /time, /auth and some
	// /order methods are actually broken if authenticated.
	if needAuth {
		req.Header.Add("X-Ovh-Timestamp", fmt.Sprintf("%d", timestamp))
		req.Header.Add("X-Ovh-Consumer", c.consumerKey)
		req.Header.Add("Accept", "application/json")

		h := sha1.New()
		h.Write([]byte(fmt.Sprintf("%s+%s+%s+%s+%s+%d",
			c.applicationSecret,
			c.consumerKey,
			method,
			target,
			body,
			timestamp,
		)))
		req.Header.Add("X-Ovh-Signature", fmt.Sprintf("$1$%x", h.Sum(nil)))
	}

	c.client.Timeout = c.Timeout
	r, err := c.client.Do(req)

	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	response := &APIResponse{}
	response.StatusCode = r.StatusCode
	response.Status = r.Status
	response.Body, err = ioutil.ReadAll(r.Body)

	return response, nil
}
