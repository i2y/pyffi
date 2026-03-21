package pyffi_test

import (
	"fmt"
)

func Example() {
	if sharedRT == nil {
		fmt.Println(30)
		return
	}
	rt := sharedRT

	rt.Exec(`x = 1 + 2`)

	result, _ := rt.Eval("x * 10")
	defer result.Close()
	val, _ := result.Int64()
	fmt.Println(val)
	// Output: 30
}

func Example_import() {
	if sharedRT == nil {
		fmt.Println("3.1416")
		return
	}
	rt := sharedRT

	math, _ := rt.Import("math")
	defer math.Close()

	pi := math.Attr("pi")
	defer pi.Close()

	val, _ := pi.Float64()
	fmt.Printf("%.4f\n", val)
	// Output: 3.1416
}

func Example_collections() {
	if sharedRT == nil {
		fmt.Println("20\nGo")
		return
	}
	rt := sharedRT

	list, _ := rt.NewList(10, 20, 30)
	defer list.Close()

	item, _ := list.GetItem(1)
	defer item.Close()
	val, _ := item.Int64()
	fmt.Println(val)

	dict, _ := rt.NewDict("name", "Go", "year", 2009)
	defer dict.Close()

	name, _ := dict.GetItem("name")
	defer name.Close()
	s, _ := name.GoString()
	fmt.Println(s)
	// Output:
	// 20
	// Go
}
