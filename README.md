# fibonacci-service

An HTTP service that returns the first `n` Fibonacci numbers as a JSON array.

```
$ make
$ ./fibonacci-service
```

```
$ curl 'http://localhost:8080/fibonacci?n=5'
[0,1,1,2,3]
$ curl 'http://localhost:8080/fibonacci?n=5&format=string'
["0","1","1","2","3"]
```

`n` is at most 79, or at most 1000 with `format=string`.

## API

### `GET /fibonacci`

| Parameter | Required | Accepted values                                       | Default  |
| --------- | -------- | ----------------------------------------------------- | -------- |
| `n`       | yes      | integer from 0 to 79, or to 1000 with `format=string` | -        |
| `format`  | no       | `number` or `string`                                  | `number` |

The 80th Fibonacci number exceeds 2^53, and a client that reads JSON numbers as float64 would round it. Above `n` = 79 the service requires `format=string`.

A reply has `Content-Type: application/json` and `Cache-Control: public, max-age=86400`.

### Errors

An invalid request gets a 400 and an [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457) problem:

```
$ curl -i 'http://localhost:8080/fibonacci?n=-1'
HTTP/1.1 400 Bad Request
Content-Length: 70
Content-Type: application/problem+json
Date: Wed, 18 Nov 2026 10:36:08 GMT

{"type":"/problems/invalid-request","detail":"n must not be negative"}
```

| `type`                                    | When                                       |
| ----------------------------------------- | ------------------------------------------ |
| `/problems/n-too-large-for-number-format` | `n` is above 79 without `format=string`    |
| `/problems/invalid-request`               | anything else; `detail` says what is wrong |

A parameter other than `n` and `format` is invalid, and so is a parameter given more than once.

`HEAD` works like `GET`. Any other method gets a 405 and an unknown path a 404, both as plain text.

### `GET /healthz`

Returns 200 with an empty body, for health checks. Not logged.

## Running

```
$ make image
$ docker run -p 8080:8080 fibonacci-service
```

The only flag is `-addr`: the listening address. The default is `:8080`.

The log is JSON on stdout:

```
{"time":"2026-11-18T11:35:33.685371+01:00","level":"INFO","msg":"request","method":"GET","path":"/fibonacci","n":"5","status":200,"duration_ms":0.087083}
{"time":"2026-11-18T11:36:08.965455+01:00","level":"INFO","msg":"request","method":"GET","path":"/fibonacci","n":"-1","status":400,"duration_ms":0.16,"problem":"/problems/invalid-request","detail":"n must not be negative","query":"n=-1"}
```

On SIGTERM or SIGINT the service stops accepting connections and gives requests in flight 5 seconds to finish.

| Exit status | When                                                                                |
| ----------- | ----------------------------------------------------------------------------------- |
| 0           | a clean stop                                                                        |
| 1           | the server failed, for example because the port was taken, or the 5 seconds ran out |
| 2           | a bad argument                                                                      |

The service sets these limits:

- request line and headers: about 12 kB
- reading the request: 5 seconds
- writing the reply: 10 seconds
- `n`: 1000, which makes the largest reply 107,543 bytes
- an idle connection: 120 seconds

It does not limit the number of simultaneous connections. Run it behind a proxy or load balancer if it is exposed to clients you do not control.

Measured in single runs in a container with 0.5 CPU and 128 MiB, by number of simultaneous connections:

| What each connection does                         | Held  | Ran out of memory |
| ------------------------------------------------- | ----- | ----------------- |
| requests the largest reply in a loop and reads it | 2,000 | 3,000             |
| requests the largest reply and never reads it     | 1,000 | 2,000             |
| makes one request, then stays idle                | 5,000 | 6,000             |
| sends an unfinished request header                | 4,000 | 6,000             |

## Development

You need Go. A Go older than the version in `go.mod` downloads that version automatically.

```
$ make build
$ make test    # uses the race detector, which needs a C compiler on Linux
$ make lint    # golangci-lint and govulncheck; needs network access
$ make image   # needs Docker
```

## Decisions

- **A bare JSON array.** It is the output the PDF shows. An object would leave room for more fields later, and [Zalando's API guidelines](https://opensource.zalando.com/restful-api-guidelines/#110) require one for that reason.
- **Two formats.** Above 79 the client must ask for strings. The service does not change format by itself. Asking explicitly means no client receives a number it would round.
- **A maximum of 1000.** 3000, the largest round value with a reply under 1 MB, was considered. No need for more than 1000 is known, and raising it later breaks no client.
- **Computed once.** The numbers never change, so the service builds the full JSON text once per format at start. For the largest reply, that serves ~20 times as many replies per second as computing per request.
- **No limit on simultaneous connections.** The limit belongs to the proxy. The table under "Running" shows the breaking points.
- **No TLS, no metrics.** TLS is left to the proxy. The request log carries status and duration, and the service has no internal state to report.
