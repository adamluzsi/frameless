# path params

- [x] path params are used both in inbound and outbound requests (Client, Router)
  - RESTClient path escapes the IDs and path params it puts into the request path, and Router unescapes them into PathParams.
- [x] router should path escape the paths it receives for registering
  - Router matches the literal path segments by their unescaped value, like http.ServeMux.
