package main

import (
	"./redigo/redis"
)

// resident memory
var (
	FoodList  []Food                   // index from 1 to FoodNum
	UserList  []string                 // index from 1 to UserNum
	UserMap   map[string]UserIdAndPass // map[name]password
	FoodNum   int
	UserNum   int
	MaxFoodID int
	MaxUserID int
	CartId    int
	CartList  []CartWL
	OrderList []int

	CacheCartId    int
	CacheFoodJson  []byte
	CacheUserLogin []int
)

// var LuaAddFood = redis.NewScript(3, `
// 		if not redis.call("HGET", KEYS[3] , '0') then
// 			if KEYS[1] - redis.call('GET', 'cart_id') > 0 then
// 				return 1
// 			end
// 			return 2
// 		end

// 		if redis.call("HGET", "orders" , KEYS[2]) then
// 			return 0
// 		end

// 		if redis.call("HINCRBY",KEYS[3],'0',ARGV[2]) > 3 then
// 			redis.call("HINCRBY",KEYS[3],'0', 0 - ARGV[2])
// 			return 3
// 		end

// 		redis.call("HINCRBY",KEYS[3],ARGV[1],ARGV[2])
// 		return 0`)

// var LuaSubmitOrder = redis.NewScript(3, `
// 		if not redis.call("HGET", KEYS[3] , '0') then
// 			if KEYS[1] - redis.call('GET', 'cart_id') > 0 then
// 				return 1
// 			end
// 			return 2
// 		end

// 		local cartItems = redis.call("HGETALL", KEYS[3])
// 		local foods = {}
// 		for i = 4, #cartItems, 2 do
// 			foods[cartItems[i-1]] = cartItems[i]
// 			if redis.call("HINCRBY", "food:" .. cartItems[i-1], "stock", 0 - cartItems[i]) < 0 then
// 				for field, value in pairs(foods) do
// 					redis.call("HINCRBY", "food:" .. field , "stock", value)
// 				end
// 				return 3
// 			end
// 		end

// 		if redis.call("HSETNX", "orders" , KEYS[2], KEYS[1]) == 0 then
// 			for field, value in pairs(foods) do
// 				redis.call("HINCRBY", "food:" .. field , "stock", value)
// 			end
// 			return 4
// 		end

var LuaSubmitOrder = redis.NewScript(3, `
		for i = 1, KEYS[3] , 2 do
			if redis.call("HINCRBY", "food:" .. ARGV[i], "stock", 0 - ARGV[i+1]) < 0 then
				for j = 1, i, 2 do
					redis.call("HINCRBY", "food:" .. ARGV[j] , "stock", ARGV[j+1])
				end
				return 1
			end
		end
		redis.call("HSET", "orders" , KEYS[1] , KEYS[2])
		return 0`)
