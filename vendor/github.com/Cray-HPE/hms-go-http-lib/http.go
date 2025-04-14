// MIT License
//
// (C) Copyright [2020-2021] Hewlett Packard Enterprise Development LP
//
// Permission is hereby granted, free of charge, to any person obtaining a
// copy of this software and associated documentation files (the "Software"),
// to deal in the Software without restriction, including without limitation
// the rights to use, copy, modify, merge, publish, distribute, sublicense,
// and/or sell copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
// THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
// OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
// OTHER DEALINGS IN THE SOFTWARE.

package hmshttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"time"

	base "github.com/Cray-HPE/hms-base/v2"

	"github.com/Cray-HPE/hms-certs/pkg/hms_certs"
	"github.com/hashicorp/go-retryablehttp"
)

// Package to slightly abstract some of the most mundane of HTTP interactions. Primary intention is as a JSON
// getter and parser, with the latter being a generic interface that can be converted to a custom structure.
type HTTPRequest struct {
	TLSClientPair       *hms_certs.HTTPClientPair // CA-capable client pair
	Client              *retryablehttp.Client     // Retryablehttp client.
	Context             context.Context           // Context to pass to the underlying HTTP client.
	MaxRetryCount       int                       // Max retry count (dflt == retryablehttp default: 4)
	MaxRetryWait        int                       // Max retry wait (dflt == retryablehttp default: 30 sec)
	FullURL             string                    // The full URL to pass to the HTTP client.
	Method              string                    // HTTP method to use.
	Payload             []byte                    // Bytes payload to pass if desired of ContentType.
	Auth                *Auth                     // Basic authentication if necessary using Auth struct.
	Timeout             time.Duration             // Timeout for entire transaction.
	ExpectedStatusCodes []int                     // Expected HTTP status return codes.
	ContentType         string                    // HTTP content type of Payload.
	CustomHeaders       map[string]string         // Custom headers to be applied to the request.
}

// NewHTTPRequest creates a new HTTPRequest with default settings.
// Note that this is kept for backward compatibility.
//
// fullURL(in): URL of the endpoint to contact.
// Return:      HTTP request descriptor.

func NewHTTPRequest(fullURL string) *HTTPRequest {
	cl, _ := hms_certs.CreateHTTPClientPair("", 30)

	return &HTTPRequest{
		TLSClientPair: cl,
		Client:        retryablehttp.NewClient(),
		Context:       context.Background(),
		FullURL:       fullURL,
		Method:        "GET",
		Payload:       nil,
		Auth:          nil,
		Timeout:       time.Duration(30) * time.Second,
		ContentType:   "application/json",
		CustomHeaders: make(map[string]string),
	}
}

// Creates a TLS cert-managed pair of HTTP clients, one that uses TLS
// certs/ca bundle and one that does not.  This is the preferred usage.
//
// fullURL(in): URL of the endpoint to contact.
// caURI(in):   URI of a CA bundle.  Can be a pathname of a file containing
//              the CA bundle, or the vault URI: hms_certs.VaultCAChainURI
//
// Return:      Ptr to HTTP request descriptor;
//              nil on success, error object on error.

func NewCAHTTPRequest(fullURL string, caURI string) (*HTTPRequest, error) {
	cl, err := hms_certs.CreateHTTPClientPair(caURI, 30)
	if err != nil {
		return &HTTPRequest{}, fmt.Errorf("ERROR creating cert-enabled HTTP transports: %v", err)
	}

	return &HTTPRequest{
		TLSClientPair: cl,
		Client:        retryablehttp.NewClient(),
		Context:       context.Background(),
		FullURL:       fullURL,
		Method:        "GET",
		Payload:       nil,
		Auth:          nil,
		Timeout:       time.Duration(30) * time.Second,
		ContentType:   "application/json",
		CustomHeaders: make(map[string]string),
	}, nil
}

func (request HTTPRequest) String() string {
	return fmt.Sprintf(
		"Client: %v, "+
			"TLSClientPair: %v, "+
			"Context: %s, "+
			"MaxRetryCount: %d, "+
			"MaxRetryWait: %d, "+
			"Method: %s, "+
			"Full URL: %s, "+
			"Payload: %s, "+
			"Auth: (%s), "+
			"Timeout: %d, "+
			"ExpectedStatusCodes: %v, "+
			"ContentType: %s",
		request.Client,
		request.TLSClientPair,
		request.Context,
		request.MaxRetryCount,
		request.MaxRetryWait,
		request.Method,
		request.FullURL,
		string(request.Payload),
		request.Auth,
		request.Timeout,
		request.ExpectedStatusCodes,
		request.ContentType)
}

// HTTP basic authentication structure.
type Auth struct {
	Username string
	Password string
}

// Custom String function to prevent passwords from being printed directly (accidentally) to output.
func (auth Auth) String() string {
	return fmt.Sprintf("Username: %s, Password: <REDACTED>", auth.Username)
}

// Given a HTTPRequest this function will facilitate the desired operation using the retryablehttp package to gracefully
// retry should the connection fail.
//
// Args:  None
// Return: payloadBytes: Raw payload of operation (if any, can be empty)
//         responseStatusCode: HTTP status code of the operation
//         err: nil on success, error object on error.

func (request *HTTPRequest) DoHTTPAction() (payloadBytes []byte, responseStatusCode int, err error) {
	// Sanity checks
	if request.FullURL == "" {
		err = fmt.Errorf("URL can not be empty")
		return
	}
	if request.Client == nil {
		err = fmt.Errorf("no retryable HTTP client set")
		return
	}

	// Setup the common HTTP request stuff.
	if request.TLSClientPair != nil {
		if request.TLSClientPair.SecureClient != nil {
			request.TLSClientPair.SecureClient.HTTPClient.Timeout = request.Timeout
		}
		if request.TLSClientPair.InsecureClient != nil {
			request.TLSClientPair.InsecureClient.HTTPClient.Timeout = request.Timeout
		}
	} else {
		request.Client.HTTPClient.Timeout = request.Timeout
	}

	var req *retryablehttp.Request

	// If there's a payload, make sure to include it.
	if request.Payload == nil {
		req, _ = retryablehttp.NewRequest(request.Method, request.FullURL, nil)
	} else {
		req, _ = retryablehttp.NewRequest(request.Method, request.FullURL, bytes.NewBuffer(request.Payload))
	}

	// Set the context to the same we were given on the way in.
	req = req.WithContext(request.Context)

	req.Header.Set("Content-Type", request.ContentType)

	// Set any custom headers.
	for headerKey, headerValue := range request.CustomHeaders {
		req.Header.Set(headerKey, headerValue)
	}

	if request.Auth != nil {
		req.SetBasicAuth(request.Auth.Username, request.Auth.Password)
	}

	//If the caller set up a TLS-aware client pair, we will try the secure one
	//first, and if the transaction fails, the insecure.  Since retryablehttp
	//default retry count is 4, and since the backoff goes to a max of 30
	//seconds, retrying will be very expensive.  Thus, the caller needs to
	//take this into account when using secure/insecure fallback, and should
	//either reduce the retry max count and/or the retry max time.
	//
	//At this point we will look at what the caller set the retry params to
	//and set the retryablehttp client accordingly.

	if request.MaxRetryCount > 0 {
		request.Client.RetryMax = request.MaxRetryCount
	}
	if request.MaxRetryWait > 0 {
		request.Client.RetryWaitMax = (time.Duration(request.MaxRetryWait) * time.Second)
	}

	//For backward compatibility, the caller can use use 'Client' as is, and
	//not use the TLSClientPair.  If that is the case the retryablehttp client
	//will use all defaults and will be cert-insecure.

	if request.TLSClientPair != nil {
		if request.TLSClientPair.SecureClient != nil {
			request.Client = request.TLSClientPair.SecureClient
		} else {
			request.Client = request.TLSClientPair.InsecureClient
		}
	}
	resp, doErr := request.Client.Do(req)
	defer base.DrainAndCloseResponseBody(resp)

	if doErr != nil {
		err = fmt.Errorf("unable to do request: %s", doErr)
		return
	}

	responseStatusCode = resp.StatusCode

	// Make sure we get the status code we expect if any are defined.

	if len(request.ExpectedStatusCodes) > 0 {
		isExpectedStatusCode := false
		for _, expectedStatusCode := range request.ExpectedStatusCodes {
			if resp.StatusCode == expectedStatusCode {
				isExpectedStatusCode = true
				break
			}
		}
		if !isExpectedStatusCode {
			err = fmt.Errorf("received unexpected status code: %d", resp.StatusCode)
		}
	}

	// Get the payload.
	payloadBytes, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		err = fmt.Errorf("unable to read response body: %s", readErr)
	}

	return
}

// Returns an interface for the response body for a given request by calling DoHTTPAction and unmarshaling.
// As such, do NOT call this method unless you expect a JSON body in return!
//
// A powerful way to use this function is by feeding its result to the mapstructure package's Decode method:
// 		v := request.GetBodyForHTTPRequest()
// 		myTypeInterface := v.(map[string]interface{})
// 		var myPopulatedStruct MyType
// 		mapstructure.Decode(myTypeInterface, &myPopulatedStruct)
// In this way you can generically make all your HTTP requests and essentially "cast" the resulting interface to a
// structure of your choosing using it as normal after that point. Just make sure to infer the correct type for `v`.
//
// Args:  None
// Return: v: Pointer to a struct to be filled with unmarshalled response
//         err: nil on success, error object on error.

func (request *HTTPRequest) GetBodyForHTTPRequest() (v interface{}, err error) {
	payloadBytes, _, err := request.DoHTTPAction()
	if err != nil {
		return
	}

	stringPayloadBytes := string(payloadBytes)
	if stringPayloadBytes != "" {
		// If we've made it to here we have all we need, unmarshal.
		jsonErr := json.Unmarshal(payloadBytes, &v)
		if jsonErr != nil {
			err = fmt.Errorf("unable to unmarshal payload: %s", jsonErr)
			return
		}
	}

	return
}
