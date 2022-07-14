package command

import (
	"encoding/json"
	"fmt"
	"github.com/go-resty/resty/v2"
	"github.com/kenlabs/pando/pkg/api/types"
	"time"
)

var Client *resty.Client
var PClientBaseURL string

func NewClient(apiBaseURL string) {
	Client = resty.New().SetBaseURL(apiBaseURL).SetDebug(false).SetTimeout(10 * time.Second)
}

func PrintResponseData(res *resty.Response) error {
	resJson := types.ResponseJson{}
	err := json.Unmarshal(res.Body(), &resJson)
	if err != nil {
		return err
	}
	prettyJson, err := json.MarshalIndent(resJson, "", " ")
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", prettyJson)
	return nil
}
