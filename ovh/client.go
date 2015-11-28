package ovh

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

// TIMEOUT api requests after 180s
const TIMEOUT = 180

// ENDPOINTS conveniently maps endpoints names to their real URI
var ENDPOINTS = map[string]string{
	"ovh-eu":        "https://eu.api.ovh.com/1.0",
	"ovh-ca":        "https://ca.api.ovh.com/1.0",
	"kimsufi-eu":    "https://eu.api.kimsufi.com/1.0",
	"kimsufi-ca":    "https://ca.api.kimsufi.com/1.0",
	"soyoustart-eu": "https://eu.api.soyoustart.com/1.0",
	"soyoustart-ca": "https://ca.api.soyoustart.com/1.0",
	"runabove-ca":   "https://api.runabove.com/1.0",
}

// Client represents an an OVH API client
type Client struct {
	endpoint          string
	applicationKey    string
	applicationSecret string
	consumerKey       string
	Timeout           int
	timeDelta         int
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

// NewClient returns an OVH API Client
func NewClient(endpoint, applicationKey, applicationSecret, consumerKey string) (c *Client, err error) {
	// Canonicalize configuration
	if !strings.Contains(endpoint, "/") {
		endpoint = ENDPOINTS[endpoint]
	}

	// Create client
	client := &Client{endpoint, applicationKey, applicationSecret, consumerKey, TIMEOUT, 0, &http.Client{}}

	// Account for clock delay with API in signatures
	timeDelta, err := client.DoGetUnAuth("/auth/time")
	if err != nil {
		return nil, err
	}

	serverTime := 0
	localTime := int(time.Now().Unix())
	err = json.Unmarshal(timeDelta.Body, &serverTime)
	if err != nil {
		return nil, err
	}
	client.timeDelta = localTime - serverTime

	return client, nil
}

//
// High level API
//

// DecodeError return error on unexpected HTTP code
func (r *APIResponse) DecodeError(expectedHTTPCode []int) (ovhResponse APIError, err error) {
	for _, code := range expectedHTTPCode {
		if r.StatusCode == code {
			return ovhResponse, nil
		}
	}

	// Decode OVH error informations from response
	if r.Body != nil {
		err := json.Unmarshal(r.Body, &ovhResponse)
		if err == nil {
			return ovhResponse, errors.New(ovhResponse.Message)
		}
	}
	return ovhResponse, fmt.Errorf("%d - %s", r.StatusCode, r.Status)
}

// DoGet Issues an authenticated get request on /path
func (c *Client) DoGet(path string) (APIResponse, error) {
	return c.Do("GET", path, nil, true)
}

// DoGetUnAuth Issues an un-authenticated get request on /path
func (c *Client) DoGetUnAuth(path string) (APIResponse, error) {
	return c.Do("GET", path, nil, false)
}

// DoPost Issues an authenticated get request on /path
func (c *Client) DoPost(path string, data interface{}) (APIResponse, error) {
	return c.Do("POST", path, data, true)
}

// DoPostUnAuth Issues an un-authenticated get request on /path
func (c *Client) DoPostUnAuth(path string, data interface{}) (APIResponse, error) {
	return c.Do("POST", path, data, false)
}

// DoPut Issues an authenticated get request on /path
func (c *Client) DoPut(path string, data interface{}) (APIResponse, error) {
	return c.Do("PUT", path, data, true)
}

// DoPutUnAuth Issues an un-authenticated get request on /path
func (c *Client) DoPutUnAuth(path string, data interface{}) (APIResponse, error) {
	return c.Do("PUT", path, data, false)
}

// DoDelete Issues an authenticated get request on /path
func (c *Client) DoDelete(path string) (APIResponse, error) {
	return c.Do("DELETE", path, nil, true)
}

// DoDeleteUnAuth Issues an un-authenticated get request on /path
func (c *Client) DoDeleteUnAuth(path string) (APIResponse, error) {
	return c.Do("DELETE", path, nil, false)
}

//
// Low Level Helpers
//

// Do calls OVH's API and signs the request if ``needAuth`` is ``true``
func (c *Client) Do(method, path string, data interface{}, needAuth bool) (response APIResponse, err error) {
	target := fmt.Sprintf("%s%s", c.endpoint, path)
	timestamp := fmt.Sprintf("%d", int(time.Now().Unix())-c.timeDelta)

	var body []byte
	if data != nil {
		body, err = json.Marshal(data)
		if err != nil {
			return response, err
		}
	}

	req, err := http.NewRequest(method, target, bytes.NewReader(body))
	if err != nil {
		return
	}

	if body != nil {
		req.Header.Add("Content-Type", "application/json;charset=utf-8")
	}
	req.Header.Add("X-Ovh-Application", c.applicationKey)

	// Some methods do not need authentication, especially /time, /auth and some
	// /order methods are actually broken if authenticated.
	if needAuth {
		req.Header.Add("X-Ovh-Timestamp", timestamp)
		req.Header.Add("X-Ovh-Consumer", c.consumerKey)
		req.Header.Add("Accept", "application/json")

		h := sha1.New()
		h.Write([]byte(fmt.Sprintf("%s+%s+%s+%s+%s+%s",
			c.applicationSecret,
			c.consumerKey,
			method,
			target,
			body,
			timestamp,
		)))
		req.Header.Add("X-Ovh-Signature", fmt.Sprintf("$1$%x", h.Sum(nil)))
	}

	c.client.Timeout = time.Duration(TIMEOUT * time.Second)
	r, err := c.client.Do(req)

	if err != nil {
		return
	}
	defer r.Body.Close()

	response.StatusCode = r.StatusCode
	response.Status = r.Status
	response.Body, err = ioutil.ReadAll(r.Body)
	return
}
