package main

import (
	"./redigo/redis"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
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
	MALFORMED_JSON_MSG      = []byte("{\"code\": \"MALFORMED_JSON\",\"message\": \"格式错误\"}")
	EMPTY_REQUEST_MSG       = []byte("{\"code\": \"EMPTY_REQUEST\",\"message\": \"请求体为空\"}")
	USER_AUTH_FAIL_MSG      = []byte("{\"code\": \"INVALID_ACCESS_TOKEN\",\"message\": \"无效的令牌\"}")
	CART_NOT_FOUND_MSG      = []byte("{\"code\": \"CART_NOT_FOUND\", \"message\": \"篮子不存在\"}")
	NOT_AUTHORIZED_CART_MSG = []byte("{\"code\": \"NOT_AUTHORIZED_TO_ACCESS_CART\",\"message\": \"无权限访问指定的篮子\"}")
	FOOD_OUT_OF_LIMIT_MSG   = []byte("{\"code\": \"FOOD_OUT_OF_LIMIT\",\"message\": \"篮子中食物数量超过了三个\"}")
	FOOD_NOT_FOUND_MSG      = []byte("{\"code\": \"FOOD_NOT_FOUND\",\"message\": \"食物不存在\"}")
	FOOD_OUT_OF_STOCK_MSG   = []byte("{\"code\": \"FOOD_OUT_OF_STOCK\", \"message\": \"食物库存不足\"}")
	ORDER_OUT_OF_LIMIT_MSG  = []byte("{\"code\": \"ORDER_OUT_OF_LIMIT\",\"message\": \"每个用户只能下一单\"}")
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
		writer.Write(MALFORMED_JSON_MSG)
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
	// Maybe the order in which two checking execute is important
	rs := Pool.Get()
	userExist, token := authorize(writer, req, rs)
	if !userExist {
		rs.Close()
		return
	}
	isEmpty, body := checkBodyEmpty(writer, req)
	if isEmpty {
		rs.Close()
		return
	}
	// transaction problem
	cartStr := strings.Split(req.URL.Path, "/")[2]
	cartId, _ := strconv.Atoi(cartStr)
	cartIdMax, err := redis.Int(rs.Do("GET", "cart_id"))
	fmt.Println("cartStr =", cartStr)
	fmt.Println("cartIdMax =", cartIdMax)
	fmt.Println("cartId =", cartId)
	if err != nil || cartId > cartIdMax || cartId < 1 {
		rs.Close()
		writer.WriteHeader(http.StatusNotFound)
		writer.Write(CART_NOT_FOUND_MSG)
		return
	}

	cartKey := "cart:" + cartStr + ":" + string(token)
	total, err := redis.Int(rs.Do("HGET", cartKey, "total"))
	if err != nil {
		rs.Close()
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(NOT_AUTHORIZED_CART_MSG)
		return
	}

	// TODO Trick: the request count is more than 0? Yes, we can checkout whether
	// total is more than 3 advanced.
	var item CartItem
	if err := json.Unmarshal(body, &item); err != nil {
		rs.Close()
		writer.WriteHeader(http.StatusBadRequest)
		writer.Write(MALFORMED_JSON_MSG)
		return
	}
	total += item.Count
	if total > 3 {
		rs.Close()
		writer.WriteHeader(http.StatusForbidden)
		writer.Write(FOOD_OUT_OF_LIMIT_MSG)
		return
	}

	// rapid test
	if item.FoodId < 1 || item.FoodId > MAXFOODID {
		rs.Close()
		writer.WriteHeader(http.StatusNotFound)
		writer.Write(FOOD_NOT_FOUND_MSG)
		return
	}
	if _, err := redis.Int(rs.Do("HEXISTS", item.FoodId, "price")); err != nil {
		rs.Close()
		writer.WriteHeader(http.StatusNotFound)
		writer.Write(FOOD_NOT_FOUND_MSG)
		return
	}
	rs.Close()
	writer.WriteHeader(http.StatusNoContent)
	return
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
		writer.Write(EMPTY_REQUEST_MSG)
		return true, nil
	} else {
		return false, tmp[:n]
	}
}
