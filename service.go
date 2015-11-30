package main

import (
	"./redigo/redis"
	"encoding/json"
	"fmt"
	"net/http"
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
)

// tuning parameters
const (
	CACHE_LEN = 73
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
)

func InitService(addr string) {
	server = http.NewServeMux()
	server.HandleFunc("/", dispacher)
	if err := http.ListenAndServe(addr, server); err != nil {
		fmt.Println(err)
	}
}

func dispacher(writer http.ResponseWriter, req *http.Request) {
	switch req.RequestURI[1] {
	case 'c':
		{
			if len(req.RequestURI) > len(CREATE_CART) {
				addFood(writer, req)
			} else {
				createCart(writer, req)
			}
		}
	case 'l':
		login(writer, req)
	case 'f':
		queryFood(writer, req)
	case 'o':
		{
			if req.Method == "POST" {
				submitOrder(writer, req)
			} else {
				queryFood(writer, req)
			}
		}
	case 'a':
		queryAllOrders(writer, req)
	}
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
	token := userIdAndPass.Id
	userId, _ := strconv.Atoi(token)
	CacheUserLogin[userId] = -1
	rs := Pool.Get()
	rs.Do("SADD", "tokens", token)
	rs.Close()
	okMsg := []byte("{\"user_id\":" + token + ",\"username\":\"" + user.Username + "\",\"access_token\":\"" + strconv.Itoa(userId+1) + "\"}")
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
	authUserIdStr := strconv.Itoa(authUserId)

	if authUserId < 1 || authUserId > MaxUserID {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(INVALID_ACCESS_TOKEN_MSG)
		return
	}

	rs := Pool.Get()
	if CacheUserLogin[authUserId] != -1 {
		if exist, _ := redis.Bool(rs.Do("SISMEMBER", "tokens", authUserIdStr)); !exist {
			rs.Close()
			writer.WriteHeader(http.StatusUnauthorized)
			writer.Write(INVALID_ACCESS_TOKEN_MSG)
			return
		}
		CacheUserLogin[authUserId] = -1
	}
	// ----------------------------------
	// END authorize

	rs.Close()
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
	authUserIdStr := strconv.Itoa(authUserId)

	if authUserId < 1 || authUserId > MaxUserID {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(INVALID_ACCESS_TOKEN_MSG)
		return
	}

	rs := Pool.Get()
	if CacheUserLogin[authUserId] != -1 {
		if exist, _ := redis.Bool(rs.Do("SISMEMBER", "tokens", authUserIdStr)); !exist {
			rs.Close()
			writer.WriteHeader(http.StatusUnauthorized)
			writer.Write(INVALID_ACCESS_TOKEN_MSG)
			return
		}
		CacheUserLogin[authUserId] = -1
	}
	// ----------------------------------
	// END authorize

	cart_id, _ := redis.Int(rs.Do("INCR", "cart_id"))

	rs.Do("HSET", "cart:"+strconv.Itoa(cart_id)+":"+authUserIdStr, TOTAL_NUM_FIELD, 0)
	rs.Close()

	writer.WriteHeader(http.StatusOK)
	writer.Write([]byte("{\"cart_id\": \"" + strconv.Itoa(cart_id) + "\"}"))
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
	authUserIdStr := strconv.Itoa(authUserId)

	if authUserId < 1 || authUserId > MaxUserID {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(INVALID_ACCESS_TOKEN_MSG)
		return
	}

	rs := Pool.Get()
	if CacheUserLogin[authUserId] != -1 {
		if exist, _ := redis.Bool(rs.Do("SISMEMBER", "tokens", authUserIdStr)); !exist {
			rs.Close()
			writer.WriteHeader(http.StatusUnauthorized)
			writer.Write(INVALID_ACCESS_TOKEN_MSG)
			return
		}
		CacheUserLogin[authUserId] = -1
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
		rs.Close()
		return
	}
	// ----------------------------------
	// END checkBodyEmpty

	var item CartItem
	if err := json.Unmarshal(body[:bodyLen], &item); err != nil {
		rs.Close()
		writer.WriteHeader(http.StatusBadRequest)
		writer.Write(MALFORMED_JSON_MSG)
		return
	}

	if item.FoodId < 1 || item.FoodId > MaxFoodID {
		rs.Close()
		writer.WriteHeader(http.StatusNotFound)
		writer.Write(FOOD_NOT_FOUND_MSG)
		return
	}

	cartIdStr := strings.Split(req.URL.Path, "/")[2]
	cartId, _ := strconv.Atoi(cartIdStr)

	//  STASRT CART_NOT_FOUND_MSG
	if cartId < 1 {
		rs.Close()
		writer.WriteHeader(http.StatusNotFound)
		writer.Write(CART_NOT_FOUND_MSG)
		return
	}

	flag, _ := redis.Int(LuaAddFood.Do(rs, cartId, authUserIdStr, "cart:"+cartIdStr+":"+authUserIdStr, item.FoodId, item.Count))
	rs.Close()

	if flag == 0 {
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	if flag == 1 {
		writer.WriteHeader(http.StatusNotFound)
		writer.Write(CART_NOT_FOUND_MSG)
		return
	}
	if flag == 2 {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(NOT_AUTHORIZED_CART_MSG)
		return
	}
	if flag == 3 {
		writer.WriteHeader(http.StatusForbidden)
		writer.Write(FOOD_OUT_OF_LIMIT_MSG)
		return
	}
}

func submitOrder(writer http.ResponseWriter, req *http.Request) {
	// START authorize
	// ----------------------------------
	req.ParseForm()
	tokenStr := req.Form.Get("access_token")
	if tokenStr == "" {
		tokenStr = req.Header.Get("Access-Token")
	}

	token, _ := strconv.Atoi(tokenStr)
	authUserId := token - 1
	authUserIdStr := strconv.Itoa(authUserId)

	if authUserId < 1 || authUserId > MaxUserID {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(INVALID_ACCESS_TOKEN_MSG)
		return
	}
	rs := Pool.Get()
	if CacheUserLogin[authUserId] != -1 {
		if exist, _ := redis.Bool(rs.Do("SISMEMBER", "tokens", authUserIdStr)); !exist {
			rs.Close()
			writer.WriteHeader(http.StatusUnauthorized)
			writer.Write(INVALID_ACCESS_TOKEN_MSG)
			return
		}
		CacheUserLogin[authUserId] = -1
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
		rs.Close()
		return
	}
	// ----------------------------------
	// END checkBodyEmpty

	var cartIdJson CartIdJson
	if err := json.Unmarshal(body[:bodyLen], &cartIdJson); err != nil {
		rs.Close()
		writer.WriteHeader(http.StatusBadRequest)
		writer.Write(MALFORMED_JSON_MSG)
		return
	}
	cartIdStr := cartIdJson.CartId

	cartId, _ := strconv.Atoi(cartIdStr)

	// copy from the same code above
	//  STASRT CART_NOT_FOUND_MSG
	if cartId < 1 {
		rs.Close()
		writer.WriteHeader(http.StatusNotFound)
		writer.Write(CART_NOT_FOUND_MSG)
		return
	}

	flag, _ := redis.Int(LuaSubmitOrder.Do(rs, cartIdStr, authUserIdStr, "cart:"+cartIdStr+":"+authUserIdStr))
	rs.Close()

	if flag == 0 {
		writer.WriteHeader(http.StatusOK)
		writer.Write([]byte("{\"id\": \"" + authUserIdStr + "\"}"))
		return
	}
	if flag == 1 {
		writer.WriteHeader(http.StatusNotFound)
		writer.Write(CART_NOT_FOUND_MSG)
		return
	}
	if flag == 2 {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(NOT_AUTHORIZED_CART_MSG)
		return
	}
	if flag == 3 {
		writer.WriteHeader(http.StatusForbidden)
		writer.Write(FOOD_OUT_OF_STOCK_MSG)
		return
	}
	if flag == 4 {
		writer.WriteHeader(http.StatusForbidden)
		writer.Write(ORDER_OUT_OF_LIMIT_MSG)
		return
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
	authUserIdStr := strconv.Itoa(authUserId)

	if authUserId < 1 || authUserId > MaxUserID {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(INVALID_ACCESS_TOKEN_MSG)
		return
	}

	rs := Pool.Get()
	if CacheUserLogin[authUserId] != -1 {
		if exist, _ := redis.Bool(rs.Do("SISMEMBER", "tokens", authUserIdStr)); !exist {
			rs.Close()
			writer.WriteHeader(http.StatusUnauthorized)
			writer.Write(INVALID_ACCESS_TOKEN_MSG)
			return
		}
		CacheUserLogin[authUserId] = -1
	}
	// ----------------------------------
	// END authorize

	cartId, err := redis.String(rs.Do("HGET", "orders", authUserIdStr))
	if err != nil {
		rs.Close()
		writer.WriteHeader(http.StatusOK)
		writer.Write([]byte("[]"))
		return
	}

	foodIdAndCounts, _ := redis.Ints(rs.Do("HGETALL", "cart:"+cartId+":"+authUserIdStr))
	rs.Close()

	var carts [1]Cart
	cart := &carts[0]
	itemNum := len(foodIdAndCounts)/2 - 1
	cart.Id = authUserIdStr
	if itemNum == 0 {
		cart.Items = []CartItem{}
	} else {
		cart.Items = make([]CartItem, itemNum)
		cnt := 0
		for i := 0; i < len(foodIdAndCounts); i += 2 {
			if foodIdAndCounts[i] != 0 {
				fid := foodIdAndCounts[i]
				quantity := foodIdAndCounts[i+1]
				cart.Items[cnt].FoodId = fid
				cart.Items[cnt].Count = quantity
				cart.TotalPrice += quantity * FoodList[fid].Price
				cnt++
			}
		}
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
	authUserIdStr := strconv.Itoa(authUserId)

	if authUserId < 1 || authUserId > MaxUserID {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(INVALID_ACCESS_TOKEN_MSG)
		return
	}

	rs := Pool.Get()
	if CacheUserLogin[authUserId] != -1 {
		if exist, _ := redis.Bool(rs.Do("SISMEMBER", "tokens", authUserIdStr)); !exist {
			rs.Close()
			writer.WriteHeader(http.StatusUnauthorized)
			writer.Write(INVALID_ACCESS_TOKEN_MSG)
			return
		}
		CacheUserLogin[authUserId] = -1
	}
	// ----------------------------------
	// END authorize

	if authUserIdStr != ROOT_TOKEN {
		rs.Close()
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(INVALID_ACCESS_TOKEN_MSG)
		return
	}

	tot, _ := redis.Int(rs.Do("HLEN", "orders"))
	carts := make([]CartDetail, tot)
	cartidAndTokens := make([]int, tot*2)
	cartidAndTokens, _ = redis.Ints(rs.Do("HGETALL", "orders"))
	cnt := 0

	for i := 0; i < tot*2; i += 2 {
		token := cartidAndTokens[i]
		carId := cartidAndTokens[i+1]
		rs.Send("HGETALL", "cart:"+strconv.Itoa(carId)+":"+strconv.Itoa(token))
	}
	rs.Flush()

	for i := 0; i < tot*2; i += 2 {

		token := cartidAndTokens[i]
		// carId := cartidAndTokens[i+1]
		// foodIdAndCounts, _ := redis.Ints(rs.Do("HGETALL", "cart:"+strconv.Itoa(carId)+":"+strconv.Itoa(token)))
		foodIdAndCounts, _ := redis.Ints(rs.Receive())

		itemNum := len(foodIdAndCounts)/2 - 1
		carts[cnt].Id = strconv.Itoa(token)
		carts[cnt].UserId = token
		if itemNum == 0 {
			carts[cnt].Items = []CartItem{}
		} else {
			carts[cnt].Items = make([]CartItem, itemNum)
			count := 0
			for j := 0; j < len(foodIdAndCounts); j += 2 {
				if foodIdAndCounts[j] != 0 {
					fid := foodIdAndCounts[j]
					carts[cnt].Items[count].FoodId = fid
					carts[cnt].Items[count].Count = foodIdAndCounts[j+1]
					carts[cnt].TotalPrice += FoodList[fid].Price * foodIdAndCounts[j+1]
					count++
				}
			}
			cnt++
		}
	}

	rs.Close()
	body, _ := json.Marshal(carts)
	writer.WriteHeader(http.StatusOK)
	writer.Write(body)
	end := time.Now().Sub(start)
	fmt.Println("queryAllOrders time: ", end.String())
}
