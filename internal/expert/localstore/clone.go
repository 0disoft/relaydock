package localstore

import (
	"encoding/json"

	"github.com/your-org/ai-runtime-gateway/internal/expert/consultation"
	"github.com/your-org/ai-runtime-gateway/internal/expert/contextpack"
	"github.com/your-org/ai-runtime-gateway/internal/expert/resultcontract"
)

func cloneConsultation(value consultation.Consultation) consultation.Consultation { return value }

func clonePack(value contextpack.Pack) contextpack.Pack {
	raw, _ := json.Marshal(value)
	var cloned contextpack.Pack
	_ = json.Unmarshal(raw, &cloned)
	return cloned
}

func cloneResult(value resultcontract.Result) resultcontract.Result {
	raw, _ := json.Marshal(value)
	var cloned resultcontract.Result
	_ = json.Unmarshal(raw, &cloned)
	return cloned
}

func cloneStoredPack(value storedPack) storedPack {
	raw, _ := json.Marshal(value)
	var cloned storedPack
	_ = json.Unmarshal(raw, &cloned)
	return cloned
}
