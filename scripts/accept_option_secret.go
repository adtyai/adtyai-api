//go:build ignore

// Options-secret accept: sensitive options land in DB as adtyenc: ciphertext,
// runtime/reload reads plaintext, forged adtyenc: input is rejected.
package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const probeKey = "AdtyAcceptProbeSecret"

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func rawDB(key string) (string, bool) {
	var o model.Option
	if err := model.DB.Where(map[string]any{"key": key}).First(&o).Error; err != nil {
		return "", false
	}
	return o.Value, true
}

func mapValue(key string) string {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return common.OptionMap[key]
}

func cleanup() {
	model.DB.Where(map[string]any{"key": probeKey}).Delete(&model.Option{})
	common.OptionMapRWMutex.Lock()
	delete(common.OptionMap, probeKey)
	common.OptionMapRWMutex.Unlock()
}

func main() {
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.IsMasterNode = true
	common.InitAdtySecretKeys()
	must(model.InitDB())
	must(model.InitLogDB())
	model.InitOptionMap()
	cleanup()
	defer cleanup()
	fmt.Println("InitDB+options OK")

	if !model.IsSensitiveOptionKey(probeKey) {
		panic("(0) probe key must be sensitive")
	}
	plain := "accept-plain-value-123"

	// (1) UpdateOption: DB ciphertext, runtime plaintext
	must(model.UpdateOption(probeKey, plain))
	raw, ok := rawDB(probeKey)
	if !ok || !strings.HasPrefix(raw, model.OptionSecretPrefix) || strings.Contains(raw, plain) {
		panic("(1) DB value must be adtyenc: ciphertext")
	}
	if mapValue(probeKey) != plain {
		panic("(1) OptionMap must hold plaintext")
	}
	fmt.Println("(1) UpdateOption DB=adtyenc: ciphertext, OptionMap plaintext OK")

	// (2) reload from DB decrypts
	common.OptionMapRWMutex.Lock()
	delete(common.OptionMap, probeKey)
	common.OptionMapRWMutex.Unlock()
	model.InitOptionMap()
	if mapValue(probeKey) != plain {
		panic("(2) reload must decrypt to plaintext")
	}
	fmt.Println("(2) reload from DB plaintext OK")

	// (3) UpdateOptionsBulk also encrypts
	plain2 := "accept-bulk-value-456"
	must(model.UpdateOptionsBulk(map[string]string{probeKey: plain2}))
	raw2, _ := rawDB(probeKey)
	if !strings.HasPrefix(raw2, model.OptionSecretPrefix) || strings.Contains(raw2, plain2) || mapValue(probeKey) != plain2 {
		panic("(3) bulk write must encrypt in DB and keep plaintext in memory")
	}
	fmt.Println("(3) UpdateOptionsBulk ciphertext OK")

	// (4) forged adtyenc: input rejected, DB/memory unchanged
	forged := model.OptionSecretPrefix + "Zm9yZ2Vk"
	for name, err := range map[string]error{
		"UpdateOption":              model.UpdateOption(probeKey, forged),
		"UpdateOptionsBulk":         model.UpdateOptionsBulk(map[string]string{probeKey: forged}),
		"ApplyBusinessParamUpdates": model.ApplyBusinessParamUpdates(map[string]string{"FirstTopupBonusRate": forged}),
	} {
		if !errors.Is(err, model.ErrOptionSecretPrefixInput) {
			panic(fmt.Sprintf("(4) %s must reject forged prefix, got %v", name, err))
		}
	}
	raw3, _ := rawDB(probeKey)
	if raw3 != raw2 || mapValue(probeKey) != plain2 {
		panic("(4) rejected write must not change DB/memory")
	}
	fmt.Println("(4) forged adtyenc: rejected, state unchanged OK")

	fmt.Println("ALL OK")
}
