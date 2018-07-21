package main

import (
	"log"
	"os"
	"sync"

	"github.com/tsocial/tessellate/server"
	"google.golang.org/grpc"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

const version = "0.0.1"

var endpoint *string

var once sync.Once
var client server.TessellateClient

func getClient() server.TessellateClient {
	once.Do(func() {
		conn, err := grpc.Dial(*endpoint, grpc.WithInsecure())
		if err != nil {
			panic(err)
		}

		client = server.NewTessellateClient(conn)
	})

	return client
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	app := kingpin.New("tessellate", "Tessellate CLI")
	endpoint = app.Flag("address", "endpoint of YourService").Short('a').Default("localhost:9977").String()
	app.Version(version)

	addWorkspaceCommand(app)
	addLayoutCommand(app)
	kingpin.MustParse(app.Parse(os.Args[1:]))
}
