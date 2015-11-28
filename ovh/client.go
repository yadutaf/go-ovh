package ovh

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

// TIMEOUT api requests after 180s
const TIMEOUT = 180

// Client represents an an OVH API client
type Client struct {
	endpoint           string
	application_key    string
	application_secret string
	consumer_key       string
	Timeout            int
	client             *http.Client
}

// ApiResponse represents a response from OVH API
type ApiResponse struct {
	StatusCode int
	Status     string
	Body       []byte
}

// ApiError represents an unmarshalled reponse from OVH in case of error
type ApiError struct {
	ErrorCode string `json:"errorCode"`
	HttpCode  string `json:"httpCode"`
	Message   string `json:"message"`
}

// NewClient returns an OVH API Client
func NewClient(endpoint, application_key, application_secret, consumer_key string) (c *Client) {
	// FIXME: stub
	return &Client{endpoint, application_key, application_secret, consumer_key, TIMEOUT, &http.Client{}}
}

//
// High level API
//

// DecodeError return error on unexpected HTTP code
func (r *ApiResponse) DecodeError(expectedHttpCode []int) (ovhResponse ApiError, err error) {
	for _, code := range expectedHttpCode {
		if r.StatusCode == code {
			return ovhResponse, nil
		}
	}

	// Try to get OVH returning info about the error
	if r.Body != nil {
		err := json.Unmarshal(r.Body, &ovhResponse)
		if err == nil {
			if len(ovhResponse.ErrorCode) != 0 {
				return ovhResponse, errors.New(ovhResponse.ErrorCode)
			} else {
				return ovhResponse, errors.New(ovhResponse.Message)
			}
		}
	}
	return ovhResponse, errors.New(fmt.Sprintf("%d - %s", r.StatusCode, r.Status))
}

// DoGet Issues an authenticated get request on /path
func (c *Client) DoGet(path string) (ApiResponse, error) {
	return c.Do("GET", path, nil, true)
}

// DoGetUnAuth Issues an un-authenticated get request on /path
func (c *Client) DoGetUnAuth(path string) (ApiResponse, error) {
	return c.Do("GET", path, nil, false)
}

// DoPost Issues an authenticated get request on /path
func (c *Client) DoPost(path string, data interface{}) (ApiResponse, error) {
	return c.Do("POST", path, data, true)
}

// DoPostUnAuth Issues an un-authenticated get request on /path
func (c *Client) DoPostUnAuth(path string, data interface{}) (ApiResponse, error) {
	return c.Do("POST", path, data, false)
}

// DoPut Issues an authenticated get request on /path
func (c *Client) DoPut(path string, data interface{}) (ApiResponse, error) {
	return c.Do("PUT", path, data, true)
}

// DoPutUnAuth Issues an un-authenticated get request on /path
func (c *Client) DoPutUnAuth(path string, data interface{}) (ApiResponse, error) {
	return c.Do("PUT", path, data, false)
}

// DoDelete Issues an authenticated get request on /path
func (c *Client) DoDelete(path string) (ApiResponse, error) {
	return c.Do("DELETE", path, nil, true)
}

// DoDeleteUnAuth Issues an un-authenticated get request on /path
func (c *Client) DoDeleteUnAuth(path string) (ApiResponse, error) {
	return c.Do("DELETE", path, nil, false)
}

//
// Low Level Helpers
//

// Call OVH's API and sign request
func (c *Client) Do(method, path string, data interface{}, need_auth bool) (response ApiResponse, err error) {
	target := fmt.Sprintf("%s%s", c.endpoint, path)
	// TODO: timedelta
	timestamp := fmt.Sprintf("%d", int32(time.Now().Unix()))

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
	req.Header.Add("X-Ovh-Application", c.application_key)

	// Some methods do not need authentication, especially /time, /auth and some
	// /order methods are actually broken if authenticated.
	if need_auth {
		req.Header.Add("X-Ovh-Timestamp", timestamp)
		req.Header.Add("X-Ovh-Consumer", c.consumer_key)
		req.Header.Add("Accept", "application/json")

		h := sha1.New()
		h.Write([]byte(fmt.Sprintf("%s+%s+%s+%s+%s+%s",
			c.application_secret,
			c.consumer_key,
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
