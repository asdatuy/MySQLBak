package main

import (
	"fmt"
	"time"
)

type rClokck struct{}

func (_ rClokck) clock() time.Time {
	return time.Now()
}

type Clock interface {
	clock() time.Time
}

func main() {
	a := rClokck{}
	fmt.Println(a.clock())
	fmt.Println(time.Now())
}
