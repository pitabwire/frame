
# Service

### About

Service is the glue that holds together components in the frame library.
All components that are to be used in the lifecycle of the application are instantiated at startup. 

### Initialization

A new service object is created by supplying the service name and or a list of [components](components) that will be utilized by the application. 
    
````go
     import(
      "github.com/pitabwire/frame"
      )
     var serviceOptions []frame.Option
     serviceName = "Notification Service"
     
          ... add code to create components
     
     service := frame.NewService(serviceName, serviceOptions ...)

````

[Components](components) can also be added by calling the init method of the app however this should be done before calling the run method. 
   
````go
serviceName = "Notification Service"
service := frame.NewService(serviceName)

var serviceOptions []frame.Option

... add code to create components to serviceOptions

service.Init(serviceOptions ...)
````

### Context usage

Once instantiated we recommend passing it around via the context object of which there are helper methods available for this purpose.


````go

ctx := context.Background()
service := frame.NewService(serviceName)

ctx := frame.ToContext(ctx, service)

// Later when required for use, we can call

service := frame.FromContext(ctx)
        
````

### Running the service
After service object is initiated we call the run method to initiate all components 
like the queues, databases and bind to the appropriate ports for the http server. 

````go
        
        service := frame.NewService(serviceName)
        err := service.Run(ctx, ":7654")
	    if err != nil {
            ...
	    }  
````

### Health checks
Once a service is running, depending on where you host it, 
its important to maintain a high level of uptime by consistently validating that service is actually available to service requests. 
Frame provides a way to check all the important sections of your system that need to be up like the database, queues or caches. 
Provide any custom implementations of validating any network components by providing a [health checker](https://pkg.go.dev/gocloud.dev/server/health) e.g.

````go
import(
	...
    "gocloud.dev/server/health/sqlhealth"
)
dbCheck := sqlhealth.New(db)
service.AddHealthCheck(dbCheck)

````

### Pre startup

In some situations we may need to execute custom code before running our application. 
Here we can add a list of functions one at a time by calling :

````go
        
service.AddPreStartMethod(func (s *Service){
    // ... custom startup code
})

````

### Cleanup
Lastly sometime you want some custom code to run at service shutdown. This is achieved by declaring some cleanup functions :

````go
        
service.AddCleanupMethod(func (){
    // ... custom startup code
})

````