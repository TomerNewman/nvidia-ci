package nvidiagpuconfig

import (
	"log"

	"github.com/kelseyhightower/envconfig"
)

// NvidiaGPUConfig contains environment information related to nvidiagpu tests.
type NvidiaGPUConfig struct {
	InstanceType  string `envconfig:"NVIDIAGPU_INSTANCE_TYPE"`
	CatalogSource string `envconfig:"NVIDIAGPU_CATALOGSOURCE"`
}

// NewNvidiaGPUConfig returns instance of NvidiaGPUConfig type.
func NewNvidiaGPUConfig() *NvidiaGPUConfig {
	log.Print("Creating new NvidiaGPUConfig")

	nvidiaGPUConfig := new(NvidiaGPUConfig)

	err := envconfig.Process("nvidiagpu_", nvidiaGPUConfig)
	if err != nil {
		log.Printf("failed to instantiate nvidiaGPUConfig: %v", err)

		return nil
	}

	return nvidiaGPUConfig
}