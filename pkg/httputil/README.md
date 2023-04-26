# httputil

This package provides a set of utilities for working with HTTP requests and responses.

## RoundTripperFunc

RoundTripperFunc is a type that allows you to create an HTTP RoundTripper 
from a function that takes an HTTP request and returns an HTTP response and an error. 
This is useful for creating custom middleware to be used with the http.Client or the http.Transport.

## RetryRoundTripper

RetryRoundTripper is a type that wraps an existing http.RoundTripper 
and provides a mechanism for retrying failed requests. 
This is useful for dealing with transient errors that can occur when making HTTP requests.

The RetryRoundTripper retries requests in case of a recoverable error
(such as network timeouts or temporary server errors) up to a certain number of times. 
If the retries exceed the maximum number of retries, the last response and error are returned.

The RetryRoundTripper considers the following errors as retriable:
- http.ErrHandlerTimeout
- net.ErrClosed, 
- and timeout errors

The RetryRoundTripper considers HTTP responses with the following status codes as temporary errors:
- Internal server error 
- Bad gateway          
- Gateway timeout      
- Service unavailable  
- Insufficient storage 
- Too many requests     
- Request timeout      
