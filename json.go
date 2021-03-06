package luajson

import (
	"encoding/json"
	"errors"
	"strconv"

	"github.com/yuin/gopher-lua"
)

// Preload adds json to the given Lua state's package.preload table. After it
// has been preloaded, it can be loaded using require:
//
//  local json = require("json")
func Preload(L *lua.LState) {
	L.PreloadModule("json", Loader)
}

// Loader is the module loader function.
func Loader(L *lua.LState) int {
	t := L.NewTable()
	L.SetFuncs(t, api)
	L.Push(t)
	return 1
}

const LibName = "cjson"

func Open(L *lua.LState) int {
	mod := L.RegisterModule("cjson", api).(*lua.LTable)
	L.Push(mod)
	return 1
}

var api = map[string]lua.LGFunction{
	"decode": apiDecode,
	"encode": apiEncode,
}

func apiDecode(L *lua.LState) int {
	str := L.CheckString(1)

	value, err := Decode(L, []byte(str))
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(value)
	return 1
}

func apiEncode(L *lua.LState) int {
	value := L.CheckAny(1)

	data, err := Encode(value)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LString(string(data)))
	return 1
}

var (
	errFunction = errors.New("cannot encode function to JSON")
	errChannel  = errors.New("cannot encode channel to JSON")
	errState    = errors.New("cannot encode state to JSON")
	errUserData = errors.New("cannot encode userdata to JSON")
	errNested   = errors.New("cannot encode recursively nested tables to JSON")
)

type jsonValue struct {
	lua.LValue
	visited map[*lua.LTable]bool
}

func (j jsonValue) MarshalJSON() ([]byte, error) {
	return encode(j.LValue, j.visited)
}

// Encode returns the JSON encoding of value.
func Encode(value lua.LValue) ([]byte, error) {
	return encode(value, make(map[*lua.LTable]bool))
}

func encode(value lua.LValue, visited map[*lua.LTable]bool) (data []byte, err error) {
	switch converted := value.(type) {
	case lua.LBool:
		data, err = json.Marshal(converted)
	case lua.LChannel:
		err = errChannel
	case lua.LNumber:
		data, err = json.Marshal(converted)
	case *lua.LFunction:
		err = errFunction
	case *lua.LNilType:
		data, err = json.Marshal(converted)
	case *lua.LState:
		err = errState
	case lua.LString:
		data, err = json.Marshal(converted)
	case *lua.LTable:
		var arr []jsonValue
		var obj map[string]jsonValue

		if visited[converted] {
			panic(errNested)
		}
		visited[converted] = true

		converted.ForEach(func(k lua.LValue, v lua.LValue) {
			i, numberKey := k.(lua.LNumber)
			if numberKey && obj == nil {
				index := int(i) - 1
				if index != len(arr) {
					// map out of order; convert to map
					obj = make(map[string]jsonValue)
					for i, value := range arr {
						obj[strconv.Itoa(i+1)] = value
					}
					obj[strconv.Itoa(index+1)] = jsonValue{v, visited}
					return
				}
				arr = append(arr, jsonValue{v, visited})
				return
			}
			if obj == nil {
				obj = make(map[string]jsonValue)
				for i, value := range arr {
					obj[strconv.Itoa(i+1)] = value
				}
			}
			obj[k.String()] = jsonValue{v, visited}
		})
		if obj != nil {
			data, err = json.Marshal(obj)
		} else {
			data, err = json.Marshal(arr)
		}
	case *lua.LUserData:
		// TODO: call metatable __tostring?
		err = errUserData
	}
	return
}

// Decode converts the JSON encoded data to Lua values.
func Decode(L *lua.LState, data []byte) (lua.LValue, error) {
	var value interface{}
	err := json.Unmarshal(data, &value)
	if err != nil {
		return nil, err
	}
	return decode(L, value), nil
}

func decode(L *lua.LState, value interface{}) lua.LValue {
	switch converted := value.(type) {
	case bool:
		return lua.LBool(converted)
	case float64:
		return lua.LNumber(converted)
	case string:
		return lua.LString(converted)
	case []interface{}:
		arr := L.CreateTable(len(converted), 0)
		for _, item := range converted {
			arr.Append(decode(L, item))
		}
		return arr
	case map[string]interface{}:
		tbl := L.CreateTable(0, len(converted))
		for key, item := range converted {
			tbl.RawSetH(lua.LString(key), decode(L, item))
		}
		return tbl
	}
	return lua.LNil
}
