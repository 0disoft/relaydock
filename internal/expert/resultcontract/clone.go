package resultcontract

import "encoding/json"

func cloneResultValue(value Result) Result {
	raw, _ := json.Marshal(value)
	var cloned Result
	_ = json.Unmarshal(raw, &cloned)
	return cloned
}
