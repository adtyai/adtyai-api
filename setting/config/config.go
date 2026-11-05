package config

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

// ConfigManager 统一管理所有配置
type ConfigManager struct {
	configs map[string]any
	mutex   sync.RWMutex
}

var GlobalConfig = NewConfigManager()

func NewConfigManager() *ConfigManager {
	return &ConfigManager{
		configs: make(map[string]any),
	}
}

// Register 注册一个配置模块
func (cm *ConfigManager) Register(name string, config any) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()
	cm.configs[name] = config
}

// Get 获取指定配置模块
func (cm *ConfigManager) Get(name string) any {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()
	return cm.configs[name]
}

// LoadFromDB 从数据库加载配置
func (cm *ConfigManager) LoadFromDB(options map[string]string) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	for name, config := range cm.configs {
		prefix := name + "."
		configMap := make(map[string]string)

		// 收集属于此配置的所有选项
		for key, value := range options {
			if after, ok := strings.CutPrefix(key, prefix); ok {
				configKey := after
				configMap[configKey] = value
			}
		}

		// 如果找到配置项，则更新配置
		if len(configMap) > 0 {
			if err := updateConfigFromMap(config, configMap); err != nil {
				common.SysError(common.LogText("failed to update config %s: %s", name, err.Error()))
				continue
			}
		}
	}

	return nil
}

// SaveToDB 将配置保存到数据库
func (cm *ConfigManager) SaveToDB(updateFunc func(key, value string) error) error {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	for name, config := range cm.configs {
		configMap, err := configToMap(config)
		if err != nil {
			return err
		}

		for key, value := range configMap {
			dbKey := name + "." + key
			if err := updateFunc(dbKey, value); err != nil {
				return err
			}
		}
	}

	return nil
}

// 辅助函数：将配置对象转换为map
func configToMap(config any) (map[string]string, error) {
	result := make(map[string]string)

	val := reflect.ValueOf(config)
	if val.Kind() == reflect.Pointer {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return nil, nil
	}

	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// 跳过未导出字段
		if !fieldType.IsExported() {
			continue
		}

		// 获取json标签作为键名
		key := fieldType.Tag.Get("json")
		if key == "" || key == "-" {
			key = fieldType.Name
		}

		// 处理不同类型的字段
		var strValue string
		switch field.Kind() {
		case reflect.String:
			strValue = field.String()
		case reflect.Bool:
			strValue = strconv.FormatBool(field.Bool())
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			strValue = strconv.FormatInt(field.Int(), 10)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			strValue = strconv.FormatUint(field.Uint(), 10)
		case reflect.Float32, reflect.Float64:
			strValue = strconv.FormatFloat(field.Float(), 'f', -1, 64)
		case reflect.Pointer:
			// 处理指针类型：如果非 nil，序列化指向的值
			if !field.IsNil() {
				bytes, err := json.Marshal(field.Interface())
				if err != nil {
					return nil, err
				}
				strValue = string(bytes)
			} else {
				// nil 指针序列化为 "null"
				strValue = "null"
			}
		case reflect.Map, reflect.Slice, reflect.Struct:
			// 复杂类型使用JSON序列化
			bytes, err := json.Marshal(field.Interface())
			if err != nil {
				return nil, err
			}
			strValue = string(bytes)
		default:
			// 跳过不支持的类型
			continue
		}

		result[key] = strValue
	}

	return result, nil
}

// 辅助函数：从map更新配置对象
func updateConfigFromMap(config any, configMap map[string]string) error {
	val := reflect.ValueOf(config)
	if val.Kind() != reflect.Pointer {
		return nil
	}
	val = val.Elem()

	if val.Kind() != reflect.Struct {
		return nil
	}

	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// 跳过未导出字段
		if !fieldType.IsExported() {
			continue
		}

		// 获取json标签作为键名
		key := fieldType.Tag.Get("json")
		if key == "" || key == "-" {
			key = fieldType.Name
		}

		// 检查map中是否有对应的值
		strValue, ok := configMap[key]
		if !ok {
			continue
		}

		// 根据字段类型设置值
		if !field.CanSet() {
			continue
		}

		if err := assignConfigField(field, key, strValue); err != nil {
			return err
		}
	}

	return nil
}

func assignConfigField(field reflect.Value, key, strValue string) error {
	if !field.CanSet() {
		return nil
	}
	switch field.Kind() {
	case reflect.String:
		field.SetString(strValue)
	case reflect.Bool:
		boolValue, err := strconv.ParseBool(strValue)
		if err != nil {
			return fmt.Errorf("invalid %s: %w", key, err)
		}
		field.SetBool(boolValue)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		intValue, err := strconv.ParseInt(strValue, 10, 64)
		if err != nil {
			floatValue, fErr := strconv.ParseFloat(strValue, 64)
			if fErr != nil || math.IsNaN(floatValue) || math.IsInf(floatValue, 0) {
				return fmt.Errorf("invalid %s: %w", key, err)
			}
			intValue = int64(floatValue)
		}
		field.SetInt(intValue)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		uintValue, err := strconv.ParseUint(strValue, 10, 64)
		if err != nil {
			floatValue, fErr := strconv.ParseFloat(strValue, 64)
			if fErr != nil || floatValue < 0 || math.IsNaN(floatValue) || math.IsInf(floatValue, 0) {
				if fErr != nil {
					err = fErr
				}
				return fmt.Errorf("invalid %s: %w", key, err)
			}
			uintValue = uint64(floatValue)
		}
		field.SetUint(uintValue)
	case reflect.Float32, reflect.Float64:
		floatValue, err := strconv.ParseFloat(strValue, 64)
		if err != nil || math.IsNaN(floatValue) || math.IsInf(floatValue, 0) {
			if err == nil {
				err = fmt.Errorf("not a finite number")
			}
			return fmt.Errorf("invalid %s: %w", key, err)
		}
		field.SetFloat(floatValue)
	case reflect.Pointer:
		if strValue == "null" {
			field.Set(reflect.Zero(field.Type()))
			return nil
		}
		// Keep a non-nil pointer. Callers such as group ratio read the original
		// map variable, not a replacement pointer.
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		if err := json.Unmarshal([]byte(strValue), field.Interface()); err != nil {
			return fmt.Errorf("invalid %s: %w", key, err)
		}
	case reflect.Map:
		fresh := reflect.New(field.Type())
		if err := json.Unmarshal([]byte(strValue), fresh.Interface()); err != nil {
			return fmt.Errorf("invalid %s: %w", key, err)
		}
		field.Set(fresh.Elem())
	case reflect.Slice, reflect.Struct:
		if err := json.Unmarshal([]byte(strValue), field.Addr().Interface()); err != nil {
			return fmt.Errorf("invalid %s: %w", key, err)
		}
	}
	return nil
}

// ConfigToMap 将配置对象转换为map（导出函数）
func ConfigToMap(config any) (map[string]string, error) {
	return configToMap(config)
}

// UpdateConfigFromMap 从map更新配置对象（导出函数）
func UpdateConfigFromMap(config any, configMap map[string]string) error {
	return updateConfigFromMap(config, configMap)
}

// ValidateConfigValue reports whether one field can be assigned without
// changing the live config. Illegal scalars must not be stored as success.
func ValidateConfigValue(config any, key, value string) error {
	val := reflect.ValueOf(config)
	if val.Kind() != reflect.Pointer || val.IsNil() {
		return nil
	}
	val = val.Elem()
	if val.Kind() != reflect.Struct {
		return nil
	}
	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		fieldType := typ.Field(i)
		if !fieldType.IsExported() {
			continue
		}
		jsonKey := fieldType.Tag.Get("json")
		if jsonKey == "" || jsonKey == "-" {
			jsonKey = fieldType.Name
		}
		if jsonKey != key {
			continue
		}
		scratch := reflect.New(fieldType.Type).Elem()
		return assignConfigField(scratch, jsonKey, value)
	}
	return nil
}

// ExportAllConfigs 导出所有已注册的配置为扁平结构
func (cm *ConfigManager) ExportAllConfigs() map[string]string {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	result := make(map[string]string)

	for name, cfg := range cm.configs {
		configMap, err := ConfigToMap(cfg)
		if err != nil {
			continue
		}

		// 使用 "模块名.配置项" 的格式添加到结果中
		for key, value := range configMap {
			result[name+"."+key] = value
		}
	}

	return result
}
