package main

import (
	"encoding/json"
	"math"
	"net/http"
)

const (
	LOGIN                 = "/login"
	QUERY_FOOD            = "/foods"
	CREATE_CART           = "/carts"
	Add_FOOD              = "/carts/"
	SUBMIT_OR_QUERY_ORDER = "/orders"
	QUERY_ALL_ORDERS      = "/admin/orders"
)

var (
	server *http.ServeMux
)

func main() {
	InitService()
}

func InitService() {
	server = http.NewServeMux()
	server.HandleFunc(LOGIN, login)
	server.HandleFunc(QUERY_FOOD, queryFood)
	server.HandleFunc(CREATE_CART, createCart)
	server.HandleFunc(Add_FOOD, addFood)
	server.HandleFunc(SUBMIT_OR_QUERY_ORDER, orderProcess)
	server.HandleFunc(QUERY_ALL_ORDERS, queryAllOrders)
	http.ListenAndServe(":8080", server)
}

func login(writer http.ResponseWriter, req *http.Request) {
	// TODO
	writer.Write([]byte(LOGIN))
}

func queryFood(writer http.ResponseWriter, req *http.Request) {
	MAXFOODID := 100
	token := req.Form.Get("access_token")
	rs := Pool.Get()
	defer rs.Close()
	foods := make([]Food, MAXFOODID)
	redis.Ints(rs.Do("HVALS", "food:"+strconv.Itoa(id)))
	for i := 1; i <= MAXFOODID; i++ {
		values, err := redis.Ints(rs.Do("HVALS", "food:"+strconv.Itoa(id)))
		if err != nil {
			break
		} else {
			foods[i] = Food{Id: i, Price: values[0], Stock: values[1]}
		}
	}
	writer.Write(json.Marshal(foods))
}

func createCart(writer http.ResponseWriter, req *http.Request) {
	// TODO
	writer.Write([]byte(CREATE_CART))
}

func addFood(writer http.ResponseWriter, req *http.Request) {
	// TODO
	writer.Write([]byte(Add_FOOD))
}

func orderProcess(writer http.ResponseWriter, req *http.Request) {
	writer.Write([]byte(SUBMIT_OR_QUERY_ORDER))
	if req.Method == "POST" {
		submitOrder(writer, req)
		// req.Method == "GET"
	} else {
		queryOneOrder(writer, req)
	}
}

func submitOrder(writer http.ResponseWriter, req *http.Request) {
	// TODO
	writer.Write([]byte("\nsubmit order"))
}

func queryOneOrder(writer http.ResponseWriter, req *http.Request) {
	// TODO
	writer.Write([]byte("\nquery an order"))
}

func queryAllOrders(writer http.ResponseWriter, req *http.Request) {
	// TODO
	writer.Write([]byte(QUERY_ALL_ORDERS))
}
