package main

import (
	"fmt"
	"time"
)

type Job struct {
	Name       string
	CreateTime time.Time
}

type rClokck struct{}

func (_ rClokck) clock() time.Time {
	return time.Now()
}

type Clock interface {
	clock() time.Time
}

func main() {
	// a := rClokck{}
	// fmt.Println(a.clock())
	// fmt.Println(time.Now())

	testStr := []int{2, 6, 8, 4, 5, 2, 0, 1}
	//sort.IntSlice.Sort(testStr)
	fmt.Print(testStr[len(testStr)-1])

	// 2025-08-11T03:52:02Z
	// layout := "2006-01-02T15:04:05Z"

	// time1, _ := time.Parse(layout, "2025-08-11T03:52:02Z")
	// time2, _ := time.Parse(layout, "2025-08-11T03:52:22Z")
	// time3, _ := time.Parse(layout, "2025-08-11T03:52:42Z")

	// JobList := []Job{
	// 	{"Job1", time3},
	// 	{"Job2", time1},
	// 	{"Job3", time2},
	// }
	// listjob := func() {
	// 	for i := range JobList {
	// 		fmt.Println(JobList[i])
	// 	}
	// }
	// listjob()

	// sort.Slice(JobList, func(i int, j int) bool {
	// 	return JobList[i].CreateTime.Before(JobList[j].CreateTime)
	// })

	// listjob()

	// fmt.Println(time1.Before(time2))

}
