package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/jaxxstorm/grass/lambdaruntime"
)

func main() {
	handler := lambdaruntime.NewHandler()
	lambda.Start(handler.Handle)
}
