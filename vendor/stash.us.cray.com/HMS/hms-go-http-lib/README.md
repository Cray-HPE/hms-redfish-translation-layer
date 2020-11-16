# HTTP Go Package

This package makes creating and using HTTP clients simpler.   It uses
the retryablehttp package to automatically do retries on failure.

There are convenience functions for creating a request object.  

NewCAHTTPRequest() creates a request object that will use a CA bundle for
TLS verification.  When using the returned object, if the TLS-secured HTTP
operation fails, it will fall back to a non-secured but still HTTPS operation.
If that fails, then the operation is considered failed.   This function can
also be used completely in insecure mode if desired, by passing an empty string
for the CA bundle URI.

NewHTTPRequest() is kept only for backwards compatibility.  It will create an
insecure-only HTTP transport/client and will work with existing implementations.

Once the request object is created, one of 2 functions can be called to carry
out the operation -- DoHTTPAction(), which returns the payload (if any) and
status code of the operation; and GetBodyForHTTPRequest(), which goes a step
further and unmarshalls the response into a passed-in data structure.



## API

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

	// HTTP basic authentication structure.
	type Auth struct {
		Username string
		Password string
	}

	//////////////////////////// FUNCTIONS ///////////////////////////////////


	// Creates a TLS cert-managed pair of HTTP clients, one that uses TLS
	// certs/ca bundle and one that does not.  This is the preferred usage.
	//
	// fullURL(in): URL of the endpoint to contact.
	// caURI(in):   URI of a CA bundle.  Can be a pathname of a file containing
	//              the CA bundle, or the vault URI: hms_certs.VaultCAChainURI
	//
	// Return:      HTTP request descriptor;
	//              nil on success, error object on error.

	func NewCAHTTPRequest(fullURL string, caURI string) (HTTPRequest,error)


	// NewHTTPRequest creates a new HTTPRequest with default settings.
	// Note that this is kept for backward compatibility.
	//
	// fullURL(in): URL of the endpoint to contact.
	// Return:      HTTP request descriptor.

	func NewHTTPRequest(fullURL string) *HTTPRequest


	// Print the contents of an HTTPRequest descriptor

	func (request HTTPRequest) String() string


	// Given a HTTPRequest this function will facilitate the desired operation 
	// using the retryablehttp package to gracefully retry should the 
	// connection fail.
	//
	// Args:   None
	// Return: payloadBytes: Raw payload of operation (if any, can be empty)
	//         responseStatusCode: HTTP status code of the operation
	//         err: nil on success, error object on error.

	func (request *HTTPRequest) DoHTTPAction() (payloadBytes []byte, 
	                                            responseStatusCode int, 
	                                            err error)


	// Returns an interface for the response body for a given request by 
	// calling DoHTTPAction and unmarshaling.  As such, do NOT call this 
	// method unless you expect a JSON body in return!
	//
	// A powerful way to use this function is by feeding its result to the 
	// mapstructure package's Decode method:
	//
	// 		v := request.GetBodyForHTTPRequest()
	// 		myTypeInterface := v.(map[string]interface{})
	// 		var myPopulatedStruct MyType
	// 		mapstructure.Decode(myTypeInterface, &myPopulatedStruct)
	//
	// In this way you can generically make all your HTTP requests and 
	// essentially "cast" the resulting interface to a structure of your 
	// choosing using it as normal after that point. Just make sure to infer 
	// the correct type for `v`.
	//
	// Args:  None
	// Return: v: Pointer to a struct to be filled with unmarshalled response
	//         err: nil on success, error object on error.

	func (request *HTTPRequest) GetBodyForHTTPRequest() (v interface{}, 
	                                                     err error)


