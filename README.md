# chatbot
a chat bot microservice

## ideas

* Chat bot will use [gRPC][grpc] for communicating between services.
* Implement new responses as functions.
* One service can have many functions.
* Chat bot will listen to chat service for messages.
* Chat bot will listen for bot functions to register.
* When messages come in chatbot will forward the requests to the bot functions
  that match.
* Chat bot will ping services to check if they are healthy. After some amount
  of unhealthy pings it will remove the service from it's list.
* Bot functions should send their trigger regular expression, endpoint, and usage/help.
* Should follow 12factor.net spec.

## todos

* integration for slack, hipchat, irc
* create rpc spec
* write tests
* lot's of other stuff

[grpc]: http://grpc.io 
