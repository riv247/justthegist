package provider

import (
	"log"
	"os"
)

var (
	logger *log.Logger
)

func init() {
	logger = log.New(os.Stdout, "[JTG] ", log.Ldate|log.Ltime|log.LUTC|log.Lshortfile)
}
