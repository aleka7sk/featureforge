package engineering

import "errors"

var (
	ErrInvalidEnvelope                 = errors.New("engineering: envelope is invalid")
	ErrUnsupportedPayloadKind          = errors.New("engineering: unsupported payload kind")
	ErrUnsupportedSchemaVersion        = errors.New("engineering: unsupported schema version")
	ErrContentTooLarge                 = errors.New("engineering: content exceeds a size bound")
	ErrInvalidContent                  = errors.New("engineering: content is invalid")
	ErrDuplicateAcceptanceCriterionKey = errors.New("engineering: duplicate acceptance criterion key")
)
