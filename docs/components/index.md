
# Components

Application objects that should be accessed fairly often in the lifecycle of an application
are created as components and supplied to the service as options during initialization.

### Available components include :

- [Server](#server) - To create a http and or a grpc server
- [Datastore](#datastore) - To link application to an sql database 
- [Queue](#queues) - Links application to external queue

## Server

Whenever a frame service is started, a server to handle requests is supposed to be supplied at service creation. 
If non is supplied a default http server that at least handles http health check requests is created.
Customizations can be done on the server by supplying a custom [driver](https://pkg.go.dev/gocloud.dev/server/driver#Server)

Available servers include :
    
- [http server](#Http server)
- [grpc server](#Grpc server)

### Http server
To create a http handler for the server, you can use [gorilla mux](https://github.com/gorilla/mux)

````go
package main
import(
	"fmt"
	"github.com/gorilla/mux"
    "github.com/pitabwire/frame"
	"log"
)

func HomeHandler(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintf(w, "Frame says yelloo!")
}

func main() {

r := mux.NewRouter()
r.HandleFunc("/", HomeHandler)

serverOption := frame.HttpHandler(r)

service := frame.NewService("Testing service",serverOption)
service.Run(ctx, ":7654")

}
````
futher customizations can also be achieved by supplying custom implementations 
Of [Server options](https://pkg.go.dev/gocloud.dev/server#Options). These can be related to 

- request logging
- Health checks
- Trace exporters and samplers

````go
package main
import(
	
"fmt"	
"github.com/pitabwire/frame"
"gocloud.dev/server/requestlog"
"os"
)

... 

reqLogger := requestlog.NewNCSALogger(os.Stdout, func(e error) { fmt.Fprintln(os.Stderr, e) })
requestLogOption := frame.HttpOptions(reqLogger)

...
````

### Grpc server 

You can utilize your grpc implementation within frame easily.
Simply declare your implementation and supply it as an option to service at startup.

````go
package main
import(
	
"fmt"	
"github.com/pitabwire/frame"
grpchello "google.golang.org/grpc/examples/helloworld/helloworld"
"os"
)

type grpcServer struct {
grpchello.UnimplementedGreeterServer
}

func (s *grpcServer) SayHello(ctx context.Context, in *grpchello.HelloRequest) (
*grpchello.HelloReply, error) {
return &grpchello.HelloReply{Message: "Hello " + in.Name + " from frame"}, nil
}

func main() {

grpcSrv := grpc.NewServer()
grpchello.RegisterGreeterServer(grpcSrv, &grpcServer{})
grpcServerOption := frame.GrpcServer(grpcSrv)
service := NewService("Testing Service Grpc", grpcServerOption)
...
}
````


## Datastore
## Queues
