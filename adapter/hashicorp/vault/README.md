# Hashicorp Adapter


## Kubernetes Secret Store + Vault

**high-level**:

```mermaid
sequenceDiagram
    participant k8s AS Kubernetes Secret Store
    participant app AS Application Service
    participant vault AS Vault

    app->>k8s: Authenticate with app namespace
    k8s-->>app: JWT token
    app->>vault: get secrets with JWT token in the HTTP request header
    vault-->>app: OK
```

**Go http.RoundTripper based pipeline**:

```mermaid
sequenceDiagram
    participant task AS Use Case
    participant vc AS Vault Client
    participant httpClient AS net/http HTTP Client
    participant krt AS http.RoundTripper with Kubernetes Secret Store Client
    participant ksc AS Kubernetes Secret Store Client
    participant dt AS http.DefaultTransport
    participant vault AS Vault


    task->>vc: Request Secret Key
    vc->>httpClient: http.Client.Do(request) <br>-> make an http request

    alt if http.Client#Transport is nil
        httpClient->>httpClient: use http.DefaultTransport for making the request
    end

    httpClient->>httpClient: prepare request, pass it to the http.Client.Transport

    httpClient->>krt: round trip begins `transport.RoundTrip(request)`

    alt if JWT token present
        krt->>krt: check if JWT token is not expired (assume it is not)
    else
        krt->>ksc: request JWT token from Kubernetes Secret Store Client
        ksc-->>krt: new JWT token
        krt->>krt: in-memory cache JWT token
    end

    krt->>krt: add JWT token to request.Header["Authenticate"]
    krt->>dt: forward http.Request

    dt->>vault: tcp over HTTP call to vault
    vault-->>dt: replies with the requested secret
    dt-->>krt: OK
    krt-->>httpClient: OK
    httpClient-->>vc: OK

    vc->>vc: interpret the response, parse the reply
    vc-->>task: reply with the requested secret
```

- you make a k8s Secret Store Client
- Then you create a http.RoundTripper that contains the next round tripper
  - by referencing the next round tripper, we can build an HTTP round tripper sequence,
    often referenced as http client middleware stack / pipeline.
- Then you inject the configured http client into the vault client
- Then when the vault client makes a request towards the Vault Server
  - the http client round trip begins, and each round tripper will call the next
