
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
    
- [http server](#http-server)
- [grpc server](#grpc-server)

### Http server
To create a http handler for the server, you can use [gorilla mux](https://github.com/gorilla/mux)

````go
package main

import (
	"fmt"
	"github.com/gorilla/mux"
	"github.com/pitabwire/frame"
	"http"
)

func HomeHandler(w http.ResponseWriter, r *http.Request) {
	_, _ = fmt.Fprintf(w, "Frame says yelloo!")
}

func main() {

	r := mux.NewRouter()
	r.HandleFunc("/", HomeHandler)

	serverOption := frame.HttpHandler(r)

	service := frame.NewService(frame.WithName("Testing service", serverOption))
	...

}
````
futher customizations can also be achieved by supplying custom implementations 
Of [Server options](https://pkg.go.dev/gocloud.dev/server#Options). These can be related to 

- request logging
- Health checks
- Trace exporters and samplers

````go
package main

import (
	"fmt"
	"github.com/pitabwire/frame"
	"gocloud.dev/server/requestlog"
	"os"
)

...

reqLogger := requestlog.NewNCSALogger(os.Stdout, func (e error) { _, _ = fmt.Fprintln(os.Stderr, e) })
requestLogOption := frame.HttpOptions(reqLogger)

...
````

### Grpc server 

You can utilize your grpc implementation within frame easily.
Simply declare your implementation and supply it as an option to service at startup.

````go
import(
    "github.com/pitabwire/frame"
    grpchello "google.golang.org/grpc/examples/helloworld/helloworld"
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
service := NewService(frame.WithName("Testing Service Grpc", grpcServerOption))
...
}
````


## Datastore

Database access is via [gorm](https://gorm.io/docs/) by default and postgres is the database of choice. 
An orm allows for easier addition of multitenant constraints removal of boiler plate code such that as a developer one does not need to think about those constraints.
[See how Tod from AWS suggests handling multitenant architecture](https://www.youtube.com/watch?v=mwQ5lipGTBI)
However there is always a performance hit taken while increasing productivity of a developer.

If you don't need to use such features, or just dont like an orm on your path. 
You can always get the raw connection and suit yourself.

Creating a database component is straight forward

````go
package main

import (
	"context"
	"github.com/pitabwire/frame"
	
)

func main() {

	ctx := context.Background()
	mainDbOption := frame.Datastore(ctx, "postgres://user:secret@primary_server/service_db", false)
	readDbOption := frame.Datastore(ctx, "postgres://user:secret@secondary_server/service_db", true)

	service := frame.NewService(frame.WithName("Data service", mainDbOption, readDbOption))

	...
}
````

Frame allows you to create multiple databases and specify whether they are read databases or write databases.
If only one database is supplied frame will utilize it for both reads and writes.

## Queues

Message queueing is essential for scalable systems as they allow the system to consume more than they can handle in an instant
and reliably process the messages later.

Currently message publishing and subscription functionality requires all queues that are going to participate to be instantiated at startup.
Out of the box the supported queues include: memory queue, nats io and gcp pubsub.

````go
package main

import (
	"context"
	"github.com/pitabwire/frame"
	
)


type messageHandler struct {}

func (m *messageHandler) Handle(ctx context.Context, message []byte) error {

	log.Printf(" Frame is handling messages: %v", string(message))
	return nil
}

func main() {

	ctx := context.Background()
	opt := RegisterPublisher("test", "mem://topicA")
	opt := RegisterSubscriber("test", "mem://topicA", 5, &messageHandler{} )
	service := frame.NewService(frame.WithName("Data service", mainDbOption, readDbOption))

	...
}
````

### Publisher:

Takes two arguments 
	- reference that is going to be used within the app
	- url to the queue that will handle the message

Utilizing the registered publisher is as shown below.

	
````go
	
...

	err =  svc.Publish(ctx, "test", []byte(" we are testing message publishing"))
...

````

###  Subscriber:

Requires four input parameters

	- A reference to be used within the app
	- A url to the queue that will listen for new messages and process them
	- Number of messages to process in parallel.
	- An interface of [message handler](https://pkg.go.dev/github.com/pitabwire/frame#SubscribeWorker) to do the actual message processing
	

*Note:* For message queue managment frame takes the traditional approach of maintaining long running connections that are subscribed. 
We however recognize that there are superior implementations like what is done with Knative
