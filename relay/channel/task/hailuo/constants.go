package hailuo

const (
	ChannelName = "hailuo-video"
	UpstreamModelMiniMaxH3 = "minimax-h3"
	ModelMiniMaxH3480P     = "minimax-h3-480p"
	ModelMiniMaxH3768P     = "minimax-h3-768p"
	ModelMiniMaxH32K       = "minimax-h3-2k"
	ModelMiniMaxH34K       = "minimax-h3-4k"
)

var ModelList = []string{
	ModelMiniMaxH3480P,
	ModelMiniMaxH3768P,
	ModelMiniMaxH32K,
	ModelMiniMaxH34K,
	"MiniMax-Hailuo-2.3",
	"MiniMax-Hailuo-2.3-Fast",
	"MiniMax-Hailuo-02",
	"T2V-01-Director",
	"T2V-01",
	"I2V-01-Director",
	"I2V-01-live",
	"I2V-01",
	"S2V-01",
}

const (
	TextToVideoEndpoint = "/v1/video_generation"
	QueryTaskEndpoint   = "/v1/query/video_generation"
)

const (
	StatusSuccess    = 0
	StatusRateLimit  = 1002
	StatusAuthFailed = 1004
	StatusNoBalance  = 1008
	StatusSensitive  = 1026
	StatusParamError = 2013
	StatusInvalidKey = 2049
)

const (
	TaskStatusPreparing  = "Preparing"
	TaskStatusQueueing   = "Queueing"
	TaskStatusProcessing = "Processing"
	TaskStatusSuccess    = "Success"
	TaskStatusFailed     = "Fail"
)

const (
	Resolution480P  = "480P"
	Resolution512P  = "512P"
	Resolution720P  = "720P"
	Resolution768P  = "768P"
	Resolution1080P = "1080P"
	Resolution2K    = "2K"
	Resolution4K    = "4K"
)

const (
	DefaultDuration   = 6
	DefaultResolution = Resolution720P
)
