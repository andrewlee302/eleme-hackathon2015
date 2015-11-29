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
	REGISTER              = "/register"
	LOGIN                 = "/login"
	QUERY_FOOD            = "/foods"
	CREATE_CART           = "/carts"
	Add_FOOD              = "/carts/"
	SUBMIT_OR_QUERY_ORDER = "/orders"
	QUERY_ALL_ORDERS      = "/admin/orders"
)

const (
	REGISTER_URL = "/register?userid="
)

const (
	TOTAL_NUM_FIELD = 0
	ROOT_TOKEN      = 1
	// NODE_NUM        = 3
)

// tuning parameters
const (
	CACHE_LEN     = 73
	POLL_INTERVAL = 100 // ms
	WAIT_INTERVAL = 10  // s
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
		log.Println(err)
		return false
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				myIp := ipnet.IP.String()
				selfHostname = myIp + ":" + selfport
				log.Printf("myIp=%s, myPort=%s\n", myIp, selfport)
				break
			}
		} else {
			log.Println("Get other ipnet:", ipnet.String())
		}
	}
	return true
}

func all2allHostname() {
	if ok := getInternalIP(); ok {
		log.Println("selfAddress:", selfHostname)
		rs := Pool.Get()
		rs.Do("SADD", "hostnames", selfHostname)
		rs.Do("INCR", "hostcount")
		rs.Close()
		for i := 0; i < 3; i++ {
			time.Sleep(WAIT_INTERVAL * time.Second)
			rs = Pool.Get()
			nodeNum, _ = redis.Int(rs.Do("GET", "hostcount"))
			orderedHostname, _ = redis.Strings(rs.Do("SMEMBERS", "hostnames"))
			rs.Close()
		}
		if len(orderedHostname) != nodeNum {
			log.Println("Unexpected exception")
		}
		sort.Strings(orderedHostname)
		log.Println("Get all hostnames.")
		log.Printf("selfIndex = %d, nodeNum = %d, hostnames = %v\n", selfIndex, nodeNum, orderedHostname)
		mode = true
	} else {
		log.Println("failed to get internal net IP")
	}
}

func InitReverseProxy() {
	all2allHostname()
	proxies = make([]*httputil.ReverseProxy, nodeNum)
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
	log.Printf("%s open the reverse proxy services\n", selfHostname)
}

func InitService(addr string) {
	InitReverseProxy()
	server = http.NewServeMux()
	server.HandleFunc(LOGIN, login)
	server.HandleFunc(REGISTER, register)
	server.HandleFunc(QUERY_FOOD, queryFood)
	server.HandleFunc(CREATE_CART, createCart)
	server.HandleFunc(Add_FOOD, addFood)
	server.HandleFunc(SUBMIT_OR_QUERY_ORDER, orderProcess)
	server.HandleFunc(QUERY_ALL_ORDERS, queryAllOrders)
	log.Printf("%s is ready to receive requests\n", selfHostname)
	if err := http.ListenAndServe(addr, server); err != nil {
		fmt.Println(err)
	}
}

func register(writer http.ResponseWriter, req *http.Request) {
	req.ParseForm()
	token := req.Form.Get("userid")
	userId, _ := strconv.Atoi(token)
	CacheUserLogin[userId] = -1
	okMsg := []byte("{\"user_id\":" + token + ",\"username\":\"" + UserList[userId] + "\",\"access_token\":\"" + strconv.Itoa(userId+1) + "\"}")
	writer.WriteHeader(http.StatusOK)
	writer.Write(okMsg)
}

func login(writer http.ResponseWriter, req *http.Request) {
	// START checkBodyEmpty
	// ----------------------------------
	var bodyLen int
	body := make([]byte, CACHE_LEN)
	if bodyLen, _ = req.Body.Read(body); bodyLen == 0 {
		writer.WriteHeader(http.StatusBadRequest)
		writer.Write(EMPTY_REQUEST_MSG)
		return
	}
	// ----------------------------------
	// END checkBodyEmpty

	var user LoginJson
	if err := json.Unmarshal(body[:bodyLen], &user); err != nil {
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
	userIdStr := userIdAndPass.Id
	userId, _ := strconv.Atoi(userIdStr)

	// partition by userId
	// =================================
	who := userId % nodeNum
	if who != selfIndex {
		reqCopy, _ := http.NewRequest("GET", REGISTER_URL+userIdStr, bytes.NewReader([]byte{}))
		proxies[who].ServeHTTP(writer, reqCopy)
		return
	}
	// =================================

	CacheUserLogin[userId] = -1
	okMsg := []byte("{\"user_id\":" + userIdStr + ",\"username\":\"" + user.Username + "\",\"access_token\":\"" + strconv.Itoa(userId+1) + "\"}")
	writer.WriteHeader(http.StatusOK)
	writer.Write(okMsg)
}

func queryFood(writer http.ResponseWriter, req *http.Request) {
	// START authorize
	// ----------------------------------
	req.ParseForm()
	tokenStr := req.Form.Get("access_token")
	if tokenStr == "" {
		tokenStr = req.Header.Get("Access-Token")
	}
	token, _ := strconv.Atoi(tokenStr)
	authUserId := token - 1
	// ======partition by userId===========
	if authUserId < 1 || authUserId > MaxUserID {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(INVALID_ACCESS_TOKEN_MSG)
		return
	}
	who := authUserId % nodeNum
	if who != selfIndex {
		proxies[who].ServeHTTP(writer, req)
		return
	}
	// =================================
	if CacheUserLogin[authUserId] != -1 {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(INVALID_ACCESS_TOKEN_MSG)
		return
	}
	// ----------------------------------
	// END authorize

	writer.WriteHeader(http.StatusOK)
	writer.Write(CacheFoodJson)
}

func createCart(writer http.ResponseWriter, req *http.Request) {
	// START authorize
	// ----------------------------------
	req.ParseForm()
	tokenStr := req.Form.Get("access_token")
	if tokenStr == "" {
		tokenStr = req.Header.Get("Access-Token")
	}
	token, _ := strconv.Atoi(tokenStr)
	authUserId := token - 1
	// ======partition by userId===========
	if authUserId < 1 || authUserId > MaxUserID {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(INVALID_ACCESS_TOKEN_MSG)
		return
	}
	who := authUserId % nodeNum
	if who != selfIndex {
		proxies[who].ServeHTTP(writer, req)
		return
	}
	// =================================
	if CacheUserLogin[authUserId] != -1 {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(INVALID_ACCESS_TOKEN_MSG)
		return
	}
	// ----------------------------------
	// END authorize

	CartId += 1
	cartId := CartId*nodeNum + selfIndex

	if cartId < UserNum+1 {
		CartList[cartId].userId = authUserId
	} else {
		var newCart CartWL
		newCart.userId = authUserId
		CartList = append(CartList, newCart)
	}
	writer.WriteHeader(http.StatusOK)
	writer.Write([]byte("{\"cart_id\": \"" + strconv.Itoa(cartId) + "\"}"))
}

func addFood(writer http.ResponseWriter, req *http.Request) {
	// START authorize
	// ----------------------------------
	req.ParseForm()
	tokenStr := req.Form.Get("access_token")
	if tokenStr == "" {
		tokenStr = req.Header.Get("Access-Token")
	}
	token, _ := strconv.Atoi(tokenStr)
	authUserId := token - 1
	// ======partition by userId===========
	if authUserId < 1 || authUserId > MaxUserID {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(INVALID_ACCESS_TOKEN_MSG)
		return
	}
	who := authUserId % nodeNum
	if who != selfIndex {
		proxies[who].ServeHTTP(writer, req)
		return
	}
	// =================================
	if CacheUserLogin[authUserId] != -1 {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(INVALID_ACCESS_TOKEN_MSG)
		return
	}
	// ----------------------------------
	// END authorize

	// START checkBodyEmpty
	// ----------------------------------
	var bodyLen int
	body := make([]byte, CACHE_LEN)
	if bodyLen, _ = req.Body.Read(body); bodyLen == 0 {
		writer.WriteHeader(http.StatusBadRequest)
		writer.Write(EMPTY_REQUEST_MSG)
		return
	}
	// ----------------------------------
	// END checkBodyEmpty

	cartIdStr := strings.Split(req.URL.Path, "/")[2]
	cartId, _ := strconv.Atoi(cartIdStr)

	if cartId < 1 {
		writer.WriteHeader(http.StatusNotFound)
		writer.Write(CART_NOT_FOUND_MSG)
		return
	}
	if CartList[cartId].userId != authUserId {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(NOT_AUTHORIZED_CART_MSG)
		return
	}
	var item CartItem
	if err := json.Unmarshal(body[:bodyLen], &item); err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		writer.Write(MALFORMED_JSON_MSG)
		return
	}
	total := CartList[cartId].total

	//have ordered
	if OrderList[authUserId] > 0 {
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
		// START authorize
		// ----------------------------------
		req.ParseForm()
		tokenStr := req.Form.Get("access_token")
		if tokenStr == "" {
			tokenStr = req.Header.Get("Access-Token")
		}
		token, _ := strconv.Atoi(tokenStr)
		authUserId := token - 1
		// ======partition by userId===========
		if authUserId < 1 || authUserId > MaxUserID {
			writer.WriteHeader(http.StatusUnauthorized)
			writer.Write(INVALID_ACCESS_TOKEN_MSG)
			return
		}
		who := authUserId % nodeNum
		if who != selfIndex {
			proxies[who].ServeHTTP(writer, req)
			return
		}
		// =================================
		if CacheUserLogin[authUserId] != -1 {
			writer.WriteHeader(http.StatusUnauthorized)
			writer.Write(INVALID_ACCESS_TOKEN_MSG)
			return
		}
		// ----------------------------------
		// END authorize

		// START checkBodyEmpty
		// ----------------------------------
		var bodyLen int
		body := make([]byte, CACHE_LEN)
		if bodyLen, _ = req.Body.Read(body); bodyLen == 0 {
			writer.WriteHeader(http.StatusBadRequest)
			writer.Write(EMPTY_REQUEST_MSG)
			return
		}
		// ----------------------------------
		// END checkBodyEmpty

		var cartIdJson CartIdJson
		if err := json.Unmarshal(body[:bodyLen], &cartIdJson); err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			writer.Write(MALFORMED_JSON_MSG)
			return
		}
		cartIdStr := cartIdJson.CartId
		cartId, _ := strconv.Atoi(cartIdStr)

		if cartId < 1 {
			writer.WriteHeader(http.StatusNotFound)
			writer.Write(CART_NOT_FOUND_MSG)
			return
		}

		cart := &CartList[cartId]
		if cart.userId != authUserId {
			writer.WriteHeader(http.StatusUnauthorized)
			writer.Write(NOT_AUTHORIZED_CART_MSG)
			return
		}

		if OrderList[authUserId] > 0 {
			writer.WriteHeader(http.StatusForbidden)
			writer.Write(ORDER_OUT_OF_LIMIT_MSG)
			return
		}

		itemNum := len(cart.Items)
		tmp := ""
		for i := 0; i < itemNum; i++ {
			if i > 0 {
				tmp = tmp + ":"
			}
			tmp = tmp + strconv.Itoa(cart.Items[i].FoodId) + ":" + strconv.Itoa(cart.Items[i].Count)
		}
		var flag int
		rs := Pool.Get()
		if itemNum == 0 {
			flag, _ = redis.Int(LuaSubmitOrder.Do(rs, authUserId, tmp, 0))
		} else if itemNum == 1 {
			flag, _ = redis.Int(LuaSubmitOrder.Do(rs, authUserId, tmp, 2, cart.Items[0].FoodId, cart.Items[0].Count))
		} else if itemNum == 2 {
			flag, _ = redis.Int(LuaSubmitOrder.Do(rs, authUserId, tmp, 4, cart.Items[0].FoodId, cart.Items[0].Count, cart.Items[1].FoodId, cart.Items[1].Count))
		} else {
			flag, _ = redis.Int(LuaSubmitOrder.Do(rs, authUserId, tmp, 6, cart.Items[0].FoodId, cart.Items[0].Count, cart.Items[1].FoodId, cart.Items[1].Count, cart.Items[2].FoodId, cart.Items[2].Count))
		}
		rs.Close()

		if flag == 0 {
			OrderList[authUserId] = cartId
			writer.WriteHeader(http.StatusOK)
			writer.Write([]byte("{\"id\": \"" + strconv.Itoa(authUserId) + "\"}"))
			return
		} else {
			writer.WriteHeader(http.StatusForbidden)
			writer.Write(FOOD_OUT_OF_STOCK_MSG)
			return
		}
	} else {
		queryOneOrder(writer, req)
	}
}

func queryOneOrder(writer http.ResponseWriter, req *http.Request) {
	// START authorize
	// ----------------------------------
	req.ParseForm()
	tokenStr := req.Form.Get("access_token")
	if tokenStr == "" {
		tokenStr = req.Header.Get("Access-Token")
	}
	token, _ := strconv.Atoi(tokenStr)
	authUserId := token - 1
	// ======partition by userId===========
	if authUserId < 1 || authUserId > MaxUserID {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(INVALID_ACCESS_TOKEN_MSG)
		return
	}
	who := authUserId % nodeNum
	if who != selfIndex {
		proxies[who].ServeHTTP(writer, req)
		return
	}
	// =================================
	if CacheUserLogin[authUserId] != -1 {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(INVALID_ACCESS_TOKEN_MSG)
		return
	}
	// ----------------------------------
	// END authorize

	cartId := OrderList[authUserId]
	if cartId == 0 {
		writer.WriteHeader(http.StatusOK)
		writer.Write([]byte("[]"))
		return
	}
	var carts [1]Cart
	cartPtr := &(carts[0])
	cartPtr.Items = CartList[cartId].Items
	cartPtr.Id = strconv.Itoa(authUserId)
	for i := 0; i < len(cartPtr.Items); i++ {
		cartPtr.TotalPrice += cartPtr.Items[i].Count * FoodList[cartPtr.Items[i].FoodId].Price
	}
	body, _ := json.Marshal(carts)
	writer.WriteHeader(http.StatusOK)
	writer.Write(body)
}

func queryAllOrders(writer http.ResponseWriter, req *http.Request) {
	start := time.Now()
	// START authorize
	// ----------------------------------
	req.ParseForm()
	tokenStr := req.Form.Get("access_token")
	if tokenStr == "" {
		tokenStr = req.Header.Get("Access-Token")
	}
	token, _ := strconv.Atoi(tokenStr)
	authUserId := token - 1
	// ======partition by userId===========
	if authUserId < 1 || authUserId > MaxUserID {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(INVALID_ACCESS_TOKEN_MSG)
		return
	}
	who := authUserId % nodeNum
	if who != selfIndex {
		proxies[who].ServeHTTP(writer, req)
		return
	}
	// =================================
	if CacheUserLogin[authUserId] != -1 {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(INVALID_ACCESS_TOKEN_MSG)
		return
	}
	// ----------------------------------
	// END authorize

	if authUserId != ROOT_TOKEN {
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
		cartPtr := &(carts[cnt])
		cartPtr.Id = orders[i]
		cartPtr.UserId, _ = strconv.Atoi(orders[i])
		foods := strings.Split(orders[i+1], ":")
		carts[cnt].Items = make([]CartItem, len(foods)/2)

		cntt := 0
		for j := 0; j < len(foods); j += 2 {
			foodId, _ := strconv.Atoi(foods[j])
			foodCount, _ := strconv.Atoi(foods[j+1])
			cartPtr.Items[cntt].FoodId = foodId
			cartPtr.Items[cntt].Count = foodCount
			cntt++
			cartPtr.TotalPrice += FoodList[foodId].Price * foodCount
		}
		cnt++
	}
	body, _ := json.Marshal(carts)
	writer.WriteHeader(http.StatusOK)
	writer.Write(body)
	end := time.Now().Sub(start)
	fmt.Println("queryAllOrders time: ", end.String())
}
