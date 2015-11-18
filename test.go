package main

import "net/http"

func main() {
	http.HandleFunc("/", hello)
	http.HandleFunc("/hello2", hello2)
	http.ListenAndServe(":8080", nil)
}

func hello(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("hello go!"))
	for i := 0; i < 5; i++ {
		//w.Write([]byte(string(i)))
		w.Write([]byte("<br/>"))
	}
}

func hello2(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("hello go!"))
	for i := 0; i < 10; i++ {
		//w.Write([]byte(string(i)))
		w.Write([]byte("<br/>"))
	}
}
