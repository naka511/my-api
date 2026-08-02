package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
)

func TestBuildTaskContentPreviewFromJSON(t *testing.T) {
	task := &model.Task{}
	task.PrivateData.SubmitRequestBody = []byte(`{"model":"video-2.0","prompt":"老虎在森林里吃饭，电影镜头"}`)

	if got := buildTaskContentPreview(task); got != "老虎在森林里吃饭，电影镜..." {
		t.Fatalf("unexpected preview: %q", got)
	}
}

func TestBuildTaskContentPreviewFromForm(t *testing.T) {
	task := &model.Task{}
	task.PrivateData.SubmitRequestBody = []byte(`model=video-2.0&prompt=%E7%8B%AE%E5%AD%90%E5%9C%A8%E5%96%9D%E9%85%92`)

	if got := buildTaskContentPreview(task); got != "狮子在喝酒" {
		t.Fatalf("unexpected preview: %q", got)
	}
}

func TestBuildTaskContentPreviewFromMultipart(t *testing.T) {
	task := &model.Task{}
	task.PrivateData.SubmitRequestBody = []byte("--boundary\r\nContent-Disposition: form-data; name=\"prompt\"\r\n\r\n无人机视角拍摄城市夜景\r\n--boundary--\r\n")

	if got := buildTaskContentPreview(task); got != "无人机视角拍摄城市夜景" {
		t.Fatalf("unexpected preview: %q", got)
	}
}
