package lfs

type BatchRequest struct {
	Operation string       `json:"operation"` // "upload" or "download"
	Transfers []string     `json:"transfers,omitempty"`
	Ref       *Ref         `json:"ref,omitempty"`
	Objects   []ObjectSpec `json:"objects"`
	HashAlgo  string       `json:"hash_algo,omitempty"`
}

type Ref struct {
	Name string `json:"name"`
}

type ObjectSpec struct {
	Oid  string `json:"oid"`
	Size int64  `json:"size"`
}

type BatchResponse struct {
	Transfer string               `json:"transfer,omitempty"`
	Objects  []ObjectResponseItem `json:"objects"`
}

type ObjectResponseItem struct {
	Oid           string                 `json:"oid"`
	Size          int64                  `json:"size"`
	Authenticated bool                   `json:"authenticated,omitempty"`
	Actions       map[string]*ActionLink `json:"actions,omitempty"`
	Error         *ObjectError           `json:"error,omitempty"`
}

type ActionLink struct {
	Href      string            `json:"href"`
	Header    map[string]string `json:"header,omitempty"`
	ExpiresIn int               `json:"expires_in,omitempty"`
}

type ObjectError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type VerifyRequest struct {
	Oid  string `json:"oid"`
	Size int64  `json:"size"`
}
