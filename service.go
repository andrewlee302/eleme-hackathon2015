package main

import (
	"./redigo/redis"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
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
	ERROR_MSG          = []byte("{\"code\": \"MALFORMED_JSON\",\"message\": \"格式错误\"}")
	EMPTY_MSG          = []byte("{\"code\": \"EMPTY_REQUEST\",\"message\": \"请求体为空\"}")
	USER_AUTH_FAIL_MSG = []byte("{\"code\": \"INVALID_ACCESS_TOKEN\",\"message\": \"无效的令牌\"}")
)

// tuning parameters
const (
	CACHE_LEN = 70
)

var (
	server *http.ServeMux
)

func InitService(addr string) {
	server = http.NewServeMux()
	server.HandleFunc(LOGIN, login)
	server.HandleFunc(QUERY_FOOD, queryFood)
	server.HandleFunc(CREATE_CART, createCart)
	server.HandleFunc(Add_FOOD, addFood)
	server.HandleFunc(SUBMIT_OR_QUERY_ORDER, orderProcess)
	server.HandleFunc(QUERY_ALL_ORDERS, queryAllOrders)
	http.ListenAndServe(addr, server)
}

func login(writer http.ResponseWriter, req *http.Request) {
	isEmpty, body := checkBodyEmpty(writer, req)
	if isEmpty {
		return
	}
	var user LoginJson
	if err := json.Unmarshal(body, &user); err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		writer.Write(ERROR_MSG)
		return
	}
	username := user.Username
	password := user.Password

	rs := Pool.Get()
	errMsg := []byte("{\"code\":\"USER_AUTH_FAIL\",\"message\":\"用户名或密码错误\"}")
	flag, _ := redis.Bool(rs.Do("HEXISTS", "user:"+username, "password"))
	// fmt.Println("flag=", flag)
	if flag == false {
		writer.WriteHeader(http.StatusForbidden)
		writer.Write(errMsg)
		return
	}

	pd, _ := redis.String(rs.Do("HGET", "user:"+username, "password"))
	// fmt.Println("pd=", pd)
	if pd != password {
		writer.WriteHeader(http.StatusForbidden)
		writer.Write(errMsg)
		return
	}

	token, _ := redis.String(rs.Do("HGET", "user:"+username, "id"))
	rs.Do("SADD", "tokens", token)
	rs.Close()
	okMsg := []byte("{\"user_id\":" + token + ",\"username\":\"" + username + "\",\"access_token\":\"" + token + "\"}")
	writer.WriteHeader(http.StatusOK)
	writer.Write(okMsg)
}

func queryFood(writer http.ResponseWriter, req *http.Request) {
	rs := Pool.Get()
	if exist, _ := authorize(writer, req, rs); !exist {
		rs.Close()
		return
	}
	foods := make([]Food, MAXFOODID)
	for i := 1; i <= MAXFOODID; i++ {
		values, err := redis.Ints(rs.Do("HVALS", "food:"+strconv.Itoa(i)))
		if err != nil {
			break
		} else {
			foods[i-1] = Food{Id: i, Price: values[0], Stock: values[1]}
		}
	}
	rs.Close()
	body, _ := json.Marshal(foods)
	writer.WriteHeader(http.StatusOK)
	writer.Write(body)
}

func createCart(writer http.ResponseWriter, req *http.Request) {
	rs := Pool.Get()
	exist, token := authorize(writer, req, rs)
	if !exist {
		rs.Close()
		return
	}
	//fmt.Println(token)
	cart_id, _ := redis.Int(rs.Do("INCR", "cart_id"))
	rs.Do("HSET", "cart:"+strconv.Itoa(cart_id)+":"+token, "total", 0)
	rs.Close()

	writer.WriteHeader(http.StatusOK)
	writer.Write([]byte("{\"cart_id\": \"" + strconv.Itoa(cart_id) + "\"}"))
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

// every action will do authorization except logining
// return the flag that indicate whether is authroized or not
func authorize(writer http.ResponseWriter, req *http.Request, rs redis.Conn) (bool, string) {
	token := req.Header.Get("Access-Token")
	if token == "" {
		token = req.Form.Get("access_token")
	}
	if exist, _ := redis.Bool(rs.Do("SISMEMBER", "tokens", token)); !exist {
		writer.WriteHeader(http.StatusForbidden)
		writer.Write(USER_AUTH_FAIL_MSG)
		fmt.Println("zheer")
		return false, ""
	}
	return true, token
}

func checkBodyEmpty(writer http.ResponseWriter, req *http.Request) (bool, []byte) {
	tmp := make([]byte, CACHE_LEN)
	if n, _ := req.Body.Read(tmp); n == 0 {
		writer.WriteHeader(http.StatusBadRequest)
		writer.Write(EMPTY_MSG)
		return true, nil
	} else {
		return false, tmp[:n]
	}
}
