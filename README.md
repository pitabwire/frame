# frame        [![Build Status](https://travis-ci.com/pitabwire/frame.svg?branch=main)](https://travis-ci.com/pitabwire/frame)

A simple bootstrap for quickly standing up small servers based on [gocloud](https://github.com/google/go-cloud) framework.

Features include:

- An http server
- A grpc server
- Database setup using [Gorm](https://github.com/go-gorm/gorm) with migrations and multitenancy support
- Easy queue publish and subscription support
- Localization
- Authentication adaptor for oauth2 and jwt access
- Authorization adaptor

The goal of this project is to simplify starting up servers with minimal boiler plate code.
All components are very pluggable with only the necessary configured items loading at runtime 
thanks to the power of go-cloud under the hood.

# Getting started:

```
    go get -u github.com/pitabwire/frame
```

# Example

````go 
import (
	"context"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/pitabwire/frame"
	"log"
	"net/http"
)

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Frame says yelloo!")
}


func main() {

	serviceName := "service_authentication"
	ctx := context.Background()

	router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/", handler)

	server := frame.HttpHandler(router)
	service := frame.NewService(serviceName,server)
	err := service.Run(ctx, ":7654")
	if err != nil {
		log.Fatal("main -- Could not run Server : %v", err)
	}

}
````

Detailed guides can be found [here](https://pitabwire.github.io/frame/) 


## development
To run tests start the docker compose file in ./tests
then run : 
````
    go test -json -cover ./...
````