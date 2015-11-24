package main

import (
	"./redigo/redis"
)

// resident memory
var (
	FoodList  []Food                   // index from 1 to FoodNum
	UserList  []User                   // index from 1 to UserNum
	UserMap   map[string]UserIdAndPass // map[name]password
	FoodNum   int
	UserNum   int
	MaxFoodID int
	MaxUserID int

	CacheFoodList []Food
	CacheCartId   int
	CacheFoodJson []byte
)

var LuaAddFood = redis.NewScript(2, `
		local RcartId = redis.call('GET', 'cart_id')
		if KEYS[1] - RcartId > 0 then
			return 1
		end

		local cartKey = 'cart:' .. KEYS[1] .. ':' .. KEYS[2]
		local Rtotal = redis.call("HGET", cartKey , '0')
		if not Rtotal then
			return 2
		end

		Rtotal = Rtotal + ARGV[2]
		if Rtotal > 3 then
			return 3 
		end

		if redis.call("GET", "order:" .. KEYS[2]) then
			return 0
		end

		redis.call("HSET",cartKey,'0',Rtotal)
		redis.call("HINCRBY",cartKey,ARGV[1],ARGV[2])
		return 0`)

var LuaSubmitOrder = redis.NewScript(2, `
		local RcartId = redis.call('GET', 'cart_id')
		if KEYS[1] - RcartId > 0 then
			return 1
		end

		local cartKey = 'cart:' .. KEYS[1] .. ':' .. KEYS[2]
		local Rtotal = redis.call("HGET", cartKey , '0')
		if not Rtotal then
			return 2
		end

		local cartItems = redis.call("HGETALL", cartKey)
		local foods = {}

		for i = 4, #cartItems, 2 do
			foods[cartItems[i-1]] = redis.call("HGET", "food:" .. cartItems[i-1], "stock") - cartItems[i]
			if foods[cartItems[i-1]] < 0 then
				return 3
			end
		end

		if redis.call("SETNX", "order:" .. KEYS[2], KEYS[1] .. ":" .. KEYS[2]) == 0 then
			return 4
		end

		for field, value in pairs(foods) do
			redis.call("HSET", "food:" .. field , "stock", value)
		end

		return 0`)
