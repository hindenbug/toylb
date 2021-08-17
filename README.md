ToyLB

```
Usage:
  --servers string
        Backends attached to the load balancer, use commas to separate
  --port int
        Serving Port
```

Running the code

```
  go build -o toylb .
  toylb --backends=http://localhost:8081,http://localhost:8082,http://localhost:8083
```

Docker Example:

```
docker-compose up
```