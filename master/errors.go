package master

import (
	"github.com/pkg/errors"
)

//master global error definitions
var (
	ErrSuc			 			= errors.New("success")
	ErrInternalError 			= errors.New("internal error")
	ErrSysBusy          		= errors.New("system busy")
	ErrParamError				= errors.New("param error")

	ErrDupDb					= errors.New("duplicated database")
	ErrDbNotExists				= errors.New("db not exists")
	ErrDupSpace					= errors.New("duplicated space")
	ErrSpaceNotExists			= errors.New("space not exists")

	ErrGenIdFailed 				= errors.New("generate id is failed")
	ErrBoltDbOpsFailed			= errors.New("boltdb operation error")
	ErrUnknownRaftCmdType 		= errors.New("unknown raft command type")

	//ErrEntryNotFound		    = errors.New("storage entry not found")

	ErrGrpcInvalidResp          = errors.New("invalid grpc response")
	ErrGrpcInvalidReq           = errors.New("invalid grpc request")
	ErrGrpcInvokeFailed			= errors.New("invoke grpc is failed")
)

// http response error code and error message definitions
const (
	ERRCODE_SUCCESS = iota
	ERRCODE_INTERNAL_ERROR
	ERRCODE_PARAM_ERROR
	ERRCODE_DUP_DB
)
var httpErrMap = map[string]int32 {
	ErrSuc:					ERRCODE_SUCCESS,
	ErrInternalError:		ERRCODE_INTERNAL_ERROR,
	ErrParamError:			ERRCODE_PARAM_ERROR,
	ErrDupDb:				ERRCODE_DUP_DB,
}
