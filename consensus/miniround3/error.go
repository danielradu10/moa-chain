package miniround3

import "errors"

var ErrNilProposedSynthesis = errors.New("miniround3: nil proposed synthesis message")
var ErrSynthesisProposalRoundKeyMismatch = errors.New("miniround3: synthesis proposal round key mismatch")
var ErrMessageNotFromLeader = errors.New("miniround3: message not from expected leader")
var ErrSynthesisBlockHashMismatch = errors.New("miniround3: synthesis block hash mismatch")
var ErrSynthesisTxCoverageMismatch = errors.New("miniround3: synthesis proposal does not cover expected eligible transactions")
