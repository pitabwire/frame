# frame        [![Build Status](https://github.com/pitabwire/frame/actions/workflows/run_tests.yml/badge.svg?branch=main)](https://github.com/pitabwire/frame/actions/workflows/run_tests.yml)   [![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/pitabwire/frame)

A simple frame for quickly setting up api servers based on [gocloud](https://github.com/google/go-cloud) framework.

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
	service := frame.NewService(frame.WithName(serviceName,server))
	err := service.Run(ctx, ":7654")
	if err != nil {
		log.Fatal("main -- Could not run Server : %v", err)
	}

}
````

Detailed guides can be found in `docs/index.md` (and on https://pitabwire.github.io/frame/).\n+\n+## Docs site (MkDocs)\n+\n+```bash\n+pip install mkdocs mkdocs-material\n+mkdocs serve\n+```\n*** End Patch"}**


## development
To run tests start the docker compose file in ./tests
then run : 
````
    go test -json -cover ./...
````
