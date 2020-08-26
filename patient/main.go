package main

import (
	"fmt"

	"github.com/broyojo/patient/patient"
)

func main() {
	/*
		p := []string{"+19087235723"}
		e := []string{"dha@xoba.com"}
	*/
	info := patient.GetInfo()
	fmt.Println(patient.Clean("(646) 241-7394"))
	fmt.Println(info)
}
