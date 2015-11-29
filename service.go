package main

import (
	"./redigo/redis"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	LOGIN                 = "/login"
	QUERY_FOOD            = "/foods"
	CREATE_CART           = "/carts"
	Add_FOOD              = "/carts/"
	SUBMIT_OR_QUERY_ORDER = "/orders"
	QUERY_ALL_ORDERS      = "/admin/orders"
)

const (
	TOTAL_NUM_FIELD = 0
	ROOT_TOKEN      = "1"
	// NODE_NUM        = 3
)

// tuning parameters
const (
	CACHE_LEN     = 73
	POLL_INTERVAL = 100 // ms
	WAIT_INTERVAL = 3   // s
)

var (
	USER_AUTH_FAIL_MSG       = []byte("{\"code\":\"USER_AUTH_FAIL\",\"message\":\"用户名或密码错误\"}")
	MALFORMED_JSON_MSG       = []byte("{\"code\": \"MALFORMED_JSON\",\"message\": \"格式错误\"}")
	EMPTY_REQUEST_MSG        = []byte("{\"code\": \"EMPTY_REQUEST\",\"message\": \"请求体为空\"}")
	INVALID_ACCESS_TOKEN_MSG = []byte("{\"code\": \"INVALID_ACCESS_TOKEN\",\"message\": \"无效的令牌\"}")
	CART_NOT_FOUND_MSG       = []byte("{\"code\": \"CART_NOT_FOUND\", \"message\": \"篮子不存在\"}")
	NOT_AUTHORIZED_CART_MSG  = []byte("{\"code\": \"NOT_AUTHORIZED_TO_ACCESS_CART\",\"message\": \"无权限访问指定的篮子\"}")
	FOOD_OUT_OF_LIMIT_MSG    = []byte("{\"code\": \"FOOD_OUT_OF_LIMIT\",\"message\": \"篮子中食物数量超过了三个\"}")
	FOOD_NOT_FOUND_MSG       = []byte("{\"code\": \"FOOD_NOT_FOUND\",\"message\": \"食物不存在\"}")
	FOOD_OUT_OF_STOCK_MSG    = []byte("{\"code\": \"FOOD_OUT_OF_STOCK\", \"message\": \"食物库存不足\"}")
	ORDER_OUT_OF_LIMIT_MSG   = []byte("{\"code\": \"ORDER_OUT_OF_LIMIT\",\"message\": \"每个用户只能下一单\"}")
)

var (
	server *http.ServeMux

	mode bool // true: reverse proxy, false: regular

	nodeNum         int
	proxies         []*httputil.ReverseProxy
	orderedHostname []string
	selfHostname    string
	selfIndex       int
)

func getInternalIP() bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		// error
		return false
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				selfHostname = ipnet.IP.String() + ":" + selfport
				break
			}
		}
	}
	return true
}

func all2allHostname() {

	fmt.Println("all2allHostname")
	if ok := getInternalIP(); ok {
		fmt.Println(selfHostname)
		rs := Pool.Get()
		rs.Do("SADD", "hostnames", selfHostname)
		rs.Do("INCR", "hostcount")
		rs.Close()
		// t := time.NewTicker(POLL_INTERVAL * time.Millisecond)
		// for {
		// 	<-t.C
		// 	count, _ := redis.Int(rs.Do("GET", "hostcount"))
		// 	fmt.Println(count)
		// 	if count == NODE_NUM {
		// 		break
		// 	}
		// }
		// t.Stop()
		for i := 0; i < 3; i++ {
			time.Sleep(WAIT_INTERVAL * time.Second)
			rs = Pool.Get()
			nodeNum, _ = redis.Int(rs.Do("GET", "hostcount"))
			orderedHostname, _ = redis.Strings(rs.Do("SMEMBERS", "hostnames"))
			rs.Close()
		}
		if len(orderedHostname) != nodeNum {
			log.Fatalln("Unexpected exception")
		}

		sort.Strings(orderedHostname)
		log.Printf("nodeNum = %d, hostnames = %v\n", nodeNum, orderedHostname)
		mode = true
	} else {
		log.Fatalln("failed to get internal net IP")
	}
}

func InitReverseProxy() {
	all2allHostname()
	proxies = make([]*httputil.ReverseProxy, nodeNum)
	fmt.Println(orderedHostname)
	for i := 0; i < nodeNum; i++ {
		ip := orderedHostname[i]
		if ip == selfHostname {
			selfIndex = i
			proxies[i] = nil
		} else {
			remote, _ := url.Parse("http://" + orderedHostname[i])
			director := func(req *http.Request) {
				req.URL.Scheme = "http"
				req.URL.Host = remote.Host
			}
			proxies[i] = &httputil.ReverseProxy{Director: director}
		}
	}
	fmt.Println(selfIndex)
}

func InitService(addr string) {
	InitReverseProxy()
	server = http.NewServeMux()
	server.HandleFunc(LOGIN, login)
	server.HandleFunc(QUERY_FOOD, queryFood)
	server.HandleFunc(CREATE_CART, createCart)
	server.HandleFunc(Add_FOOD, addFood)
	server.HandleFunc(SUBMIT_OR_QUERY_ORDER, orderProcess)
	server.HandleFunc(QUERY_ALL_ORDERS, queryAllOrders)
	if err := http.ListenAndServe(addr, server); err != nil {
		fmt.Println(err)
	}
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
	userIdAndPass, ok := UserMap[user.Username]
	if !ok || userIdAndPass.Password != user.Password {
		writer.WriteHeader(http.StatusForbidden)
		writer.Write(USER_AUTH_FAIL_MSG)
		return
	}
	token := userIdAndPass.Id
	userId, _ := strconv.Atoi(token)

	// partition by userId
	// =================================
	who := userId % nodeNum
	if who != selfIndex {
		reqCopy, _ := http.NewRequest(req.Method, req.URL.String(), bytes.NewReader(body))
		reqCopy.Header = req.Header
		proxies[who].ServeHTTP(writer, reqCopy)
		return
	}
	// =================================

	CacheUserLogin[userId] = -1
	okMsg := []byte("{\"user_id\":" + token + ",\"username\":\"" + user.Username + "\",\"access_token\":\"" + strconv.Itoa(userId+1) + "\"}")
	writer.WriteHeader(http.StatusOK)
	writer.Write(okMsg)
}

func queryFood(writer http.ResponseWriter, req *http.Request) {
	if exist, _ := authorize(writer, req); !exist {
		return
	}
	writer.WriteHeader(http.StatusOK)
	writer.Write(CacheFoodJson)
}

func createCart(writer http.ResponseWriter, req *http.Request) {
	exist, token := authorize(writer, req)
	if !exist {
		return
	}
	CartId += 1
	cartId := CartId
	CartList[cartId].userId, _ = strconv.Atoi(token)
	writer.WriteHeader(http.StatusOK)
	writer.Write([]byte("{\"cart_id\": \"" + strconv.Itoa(cartId) + "\"}"))
}

func addFood(writer http.ResponseWriter, req *http.Request) {
	userExist, token := authorize(writer, req)
	if !userExist {
		return
	}
	isEmpty, body := checkBodyEmpty(writer, req)
	if isEmpty {
		return
	}
	// transaction problem
	cartIdStr := strings.Split(req.URL.Path, "/")[2]
	cartId, _ := strconv.Atoi(cartIdStr)

	if cartId > CartId || cartId < 1 {
		writer.WriteHeader(http.StatusNotFound)
		writer.Write(CART_NOT_FOUND_MSG)
		return
	}

	userId, _ := strconv.Atoi(token)
	if CartList[cartId].userId != userId {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(NOT_AUTHORIZED_CART_MSG)
		return
	}

	var item CartItem
	if err := json.Unmarshal(body, &item); err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		writer.Write(MALFORMED_JSON_MSG)
		return
	}

	total := CartList[cartId].total

	//have ordered
	if OrderList[userId] > 0 {
		writer.WriteHeader(http.StatusNoContent)
		return
	}

	total += item.Count
	if total > 3 {
		writer.WriteHeader(http.StatusForbidden)
		writer.Write(FOOD_OUT_OF_LIMIT_MSG)
		return
	}

	if item.FoodId < 1 || item.FoodId > MaxFoodID {
		writer.WriteHeader(http.StatusNotFound)
		writer.Write(FOOD_NOT_FOUND_MSG)
		return
	}

	CartList[cartId].total = total

	tag := true
	for i := 0; i < len(CartList[cartId].Items); i++ {
		if CartList[cartId].Items[i].FoodId == item.FoodId {
			CartList[cartId].Items[i].Count += item.Count
			tag = false
			break
		}
	}

	if tag {
		CartList[cartId].Items = append(CartList[cartId].Items, item)
	}

	writer.WriteHeader(http.StatusNoContent)
	return

}

func orderProcess(writer http.ResponseWriter, req *http.Request) {
	if req.Method == "POST" {
		submitOrder(writer, req)
	} else {
		queryOneOrder(writer, req)
	}
}

func submitOrder(writer http.ResponseWriter, req *http.Request) {

	userExist, token := authorize(writer, req)
	if !userExist {
		return
	}
	isEmpty, body := checkBodyEmpty(writer, req)
	if isEmpty {
		return
	}

	var cartIdJson CartIdJson
	if err := json.Unmarshal(body, &cartIdJson); err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		writer.Write(MALFORMED_JSON_MSG)
		return
	}
	cartIdStr := cartIdJson.CartId
	cartId, _ := strconv.Atoi(cartIdStr)

	if cartId < 1 || cartId > CartId {
		writer.WriteHeader(http.StatusNotFound)
		writer.Write(CART_NOT_FOUND_MSG)
		return
	}

	userId, _ := strconv.Atoi(token)
	if CartList[cartId].userId != userId {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(NOT_AUTHORIZED_CART_MSG)
		return
	}

	if OrderList[userId] > 0 {
		writer.WriteHeader(http.StatusForbidden)
		writer.Write(ORDER_OUT_OF_LIMIT_MSG)
		return
	}

	//redis submitorder
	rs := Pool.Get()

	flag := 0
	var err error
	itemNum := len(CartList[cartId].Items)

	tmp := ""
	for i := 0; i < itemNum; i++ {
		tmp = tmp + strconv.Itoa(CartList[cartId].Items[i].FoodId) + ":" + strconv.Itoa(CartList[cartId].Items[i].Count) + ":"
	}

	if itemNum == 0 {
		flag, _ = redis.Int(LuaSubmitOrder.Do(rs, token, tmp, 0))
	} else if itemNum == 1 {
		flag, err = redis.Int(LuaSubmitOrder.Do(rs, token, tmp, 2, CartList[cartId].Items[0].FoodId, CartList[cartId].Items[0].Count))
		if err != nil {
			fmt.Println(err)
		}
	} else if itemNum == 2 {
		flag, _ = redis.Int(LuaSubmitOrder.Do(rs, token, tmp, 4, CartList[cartId].Items[0].FoodId, CartList[cartId].Items[0].Count, CartList[cartId].Items[1].FoodId, CartList[cartId].Items[1].Count))
	} else {
		flag, _ = redis.Int(LuaSubmitOrder.Do(rs, token, tmp, 6, CartList[cartId].Items[0].FoodId, CartList[cartId].Items[0].Count, CartList[cartId].Items[1].FoodId, CartList[cartId].Items[1].Count, CartList[cartId].Items[2].FoodId, CartList[cartId].Items[2].Count))
	}
	rs.Close()

	if flag == 0 {
		OrderList[userId] = cartId
		writer.WriteHeader(http.StatusOK)
		writer.Write([]byte("{\"id\": \"" + token + "\"}"))
		return
	} else {
		writer.WriteHeader(http.StatusForbidden)
		writer.Write(FOOD_OUT_OF_STOCK_MSG)
		return
	}

	// cartKey := "cart:" + cartIdStr + ":" + token
	// _, cartExistErr := redis.Int(rs.Do("HGET", cartKey, TOTAL_NUM_FIELD))
	// if cartExistErr != nil {
	// 	rs.Close()
	// 	writer.WriteHeader(http.StatusUnauthorized)
	// 	writer.Write(NOT_AUTHORIZED_CART_MSG)
	// 	//fmt.Println(string(NOT_AUTHORIZED_CART_MSG))
	// 	return
	// }

	// // transaction problem
	// foodIdAndCounts, _ := redis.Ints(rs.Do("HGETALL", cartKey))
	// var cart Cart
	// itemNum := len(foodIdAndCounts)/2 - 1
	// //fmt.Println("itemNum =", itemNum)
	// if itemNum == 0 {
	// 	cart.Items = []CartItem{}
	// } else {
	// 	cart.Items = make([]CartItem, itemNum)
	// 	cnt := 0
	// 	for i := 0; i < len(foodIdAndCounts); i += 2 {
	// 		if foodIdAndCounts[i] != TOTAL_NUM_FIELD {
	// 			cart.Items[cnt].FoodId = foodIdAndCounts[i]
	// 			cart.Items[cnt].Count = foodIdAndCounts[i+1]
	// 			cnt++
	// 			//fmt.Println("foodId, reqCount =", foodIdAndCounts[i], foodIdAndCounts[i+1])
	// 		}
	// 	}
	// }
	// for i := 0; i < len(cart.Items); i++ {
	// 	stock, _ := redis.Int(rs.Do("HGET", "food:"+strconv.Itoa(cart.Items[i].FoodId), "stock"))
	// 	tmp := stock - cart.Items[i].Count
	// 	cart.Items[i].Count = tmp
	// 	//fmt.Println("stock, reqCount = ", stock, cart.Items[i].Count)
	// 	if tmp < 0 {
	// 		rs.Close()
	// 		writer.WriteHeader(http.StatusForbidden)
	// 		writer.Write(FOOD_OUT_OF_STOCK_MSG)
	// 		//fmt.Println(string(FOOD_OUT_OF_STOCK_MSG))
	// 		return
	// 	}
	// }

	// // no transaction problem

	// if tag, _ := redis.Bool(rs.Do("HEXISTS", "orders", token)); tag {
	// 	rs.Close()
	// 	writer.WriteHeader(http.StatusForbidden)
	// 	writer.Write(ORDER_OUT_OF_LIMIT_MSG)
	// 	return
	// }

	// isSuccess, _ := redis.Int(rs.Do("HSETNX", "orders", token, cartIdStr))
	// //fmt.Println("SETNX", "order:"+token, cartIdStr+":"+token)
	// //fmt.Println("isSuccess =", isSuccess)
	// if isSuccess == 0 {
	// 	rs.Close()
	// 	writer.WriteHeader(http.StatusForbidden)
	// 	writer.Write(ORDER_OUT_OF_LIMIT_MSG)
	// 	//fmt.Println(string(ORDER_OUT_OF_LIMIT_MSG))
	// 	return
	// }

	// for i := 0; i < len(cart.Items); i++ {
	// 	rs.Do("HSET", "food:"+strconv.Itoa(cart.Items[i].FoodId), "stock", cart.Items[i].Count)
	// 	//fmt.Println("food:"+strconv.Itoa(cart.Items[i].FoodId), "stock", cart.Items[i].Count)
	// }
	// rs.Close()
	// writer.WriteHeader(http.StatusOK)
	// writer.Write([]byte("{\"id\": \"" + token + "\"}"))
	// //fmt.Println("order success")
	// return

	// var flag int
	// if cartId > CacheCartId {
	// 	flags, err := redis.Ints(LuaSubmitOrder.Do(rs, cartIdStr, token))
	// 	if err != nil {
	// 		fmt.Println(err)
	// 	}
	// 	flag = flags[0]
	// 	CacheCartId = flags[1]
	// } else {
	// 	flag, _ = redis.Int(LuaSubmitOrderWithoutCartId.Do(rs, cartIdStr, token))
	// }

	// flag, _ := redis.Int(LuaSubmitOrder.Do(rs, cartIdStr, token, "cart:"+cartIdStr+":"+token))
	// rs.Close()

	// if flag == 0 {
	// 	writer.WriteHeader(http.StatusOK)
	// 	writer.Write([]byte("{\"id\": \"" + token + "\"}"))
	// 	return
	// }
	// if flag == 1 {
	// 	writer.WriteHeader(http.StatusNotFound)
	// 	writer.Write(CART_NOT_FOUND_MSG)
	// 	return
	// }
	// if flag == 2 {
	// 	writer.WriteHeader(http.StatusUnauthorized)
	// 	writer.Write(NOT_AUTHORIZED_CART_MSG)
	// 	return
	// }
	// if flag == 3 {
	// 	writer.WriteHeader(http.StatusForbidden)
	// 	writer.Write(FOOD_OUT_OF_STOCK_MSG)
	// 	return
	// }
	// if flag == 4 {
	// 	writer.WriteHeader(http.StatusForbidden)
	// 	writer.Write(ORDER_OUT_OF_LIMIT_MSG)
	// 	return
	// }
	// //script version end

}

func queryOneOrder(writer http.ResponseWriter, req *http.Request) {
	exist, token := authorize(writer, req)
	if !exist {
		return
	}

	userId, _ := strconv.Atoi(token)
	cartId := OrderList[userId]
	if cartId == 0 {
		writer.WriteHeader(http.StatusOK)
		writer.Write([]byte("[]"))
		return
	}

	var carts [1]Cart
	carts[0].Items = CartList[cartId].Items
	carts[0].Id = token
	for i := 0; i < len(carts[0].Items); i++ {
		carts[0].TotalPrice += carts[0].Items[i].Count * FoodList[carts[0].Items[i].FoodId].Price
	}

	body, _ := json.Marshal(carts)
	// fmt.Println(string(body))
	writer.WriteHeader(http.StatusOK)
	writer.Write(body)
}

func queryAllOrders(writer http.ResponseWriter, req *http.Request) {
	start := time.Now()

	exist, token := authorize(writer, req)
	if !exist {
		return
	}

	if token != ROOT_TOKEN {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(INVALID_ACCESS_TOKEN_MSG)
		return
	}

	rs := Pool.Get()
	tot, _ := redis.Int(rs.Do("HLEN", "orders"))
	carts := make([]CartDetail, tot)
	orders, _ := redis.Strings(rs.Do("HGETALL", "orders"))
	rs.Close()
	cnt := 0
	for i := 0; i < tot*2; i += 2 {
		carts[cnt].Id = orders[i]
		carts[cnt].UserId, _ = strconv.Atoi(orders[i])
		foods := strings.Split(orders[i+1], ":")
		carts[cnt].Items = make([]CartItem, len(foods)/2)

		cntt := 0
		for j := 0; j < len(foods)-1; j += 2 {
			foodId, _ := strconv.Atoi(foods[j])
			foodCount, _ := strconv.Atoi(foods[j+1])
			carts[cnt].Items[cntt].FoodId = foodId
			carts[cnt].Items[cntt].Count = foodCount
			cntt++
			carts[cnt].TotalPrice += FoodList[foodId].Price * foodCount
		}
		cnt++
	}
	body, _ := json.Marshal(carts)
	writer.WriteHeader(http.StatusOK)
	writer.Write(body)
	end := time.Now().Sub(start)
	fmt.Println("queryAllOrders time: ", end.String())
}

// every action will do authorization except logining
// return the flag that indicate whether is authroized or not
func authorize(writer http.ResponseWriter, req *http.Request) (bool, string) {
	req.ParseForm()
	token := req.Form.Get("access_token")
	if token == "" {
		token = req.Header.Get("Access-Token")
	}

	userId, _ := strconv.Atoi(token)
	userId -= 1

	// partition by userId
	// =================================
	who := userId % nodeNum
	if who != selfIndex {
		proxies[who].ServeHTTP(writer, req)
		return false, ""
	}
	// =================================
	// log.Println(req.Host, req.URL.String())

	// fmt.Println(userId)

	token = strconv.Itoa(userId)

	if userId < 1 || userId > MaxUserID || CacheUserLogin[userId] != -1 {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(INVALID_ACCESS_TOKEN_MSG)
		return false, ""
	} else {
		return true, token
	}
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
