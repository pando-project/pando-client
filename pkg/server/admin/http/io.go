package adminserver

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
)

func (req *ImportFileReq) ReadFrom(r io.Reader) (int64, error) {
	return unmarshalAsJson(r, req)
}

func (req *SyncReq) ReadFrom(r io.Reader) (int64, error) {
	return unmarshalAsJson(r, req)
}

func (req *ImportFileRes) WriteTo(w io.Writer) (int64, error) {
	return marshalToJson(w, req)
}

func (res *ResponseJson) WriteTo(w io.Writer) (int64, error) {
	return marshalToJson(w, res)
}

func unmarshalAsJson(r io.Reader, dst interface{}) (int64, error) {
	body, err := ioutil.ReadAll(r)
	if err != nil {
		return 0, err
	}
	return int64(len(body)), json.Unmarshal(body, dst)
}

func marshalToJson(w io.Writer, src interface{}) (int64, error) {
	body, err := json.Marshal(src)
	if err != nil {
		return 0, err
	}
	written, err := w.Write(body)
	return int64(written), err
}

func respond(w http.ResponseWriter, statusCode int, body io.WriterTo) {
	w.WriteHeader(statusCode)
	// Attempt to serialize body as JSON
	if _, err := body.WriteTo(w); err != nil {
		logger.Errorw("faild to write response ", "err", err)
		return
	}
}
