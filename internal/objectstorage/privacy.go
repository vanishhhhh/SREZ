package objectstorage

import (
	"context"
	"errors"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

var ErrUnsafeBucket = errors.New("bucket privacy cannot be verified")

// Accept only owner canonical ACL grants. Missing/unsupported ACL inspection is
// not a reason to assume privacy. In particular AllUsers/AuthenticatedUsers
// groups and non-owner grants are rejected, regardless of the permission.
func (s *S3) verifyACL(ctx context.Context) error {
	a, e := s.client.GetBucketAcl(ctx, &s3.GetBucketAclInput{Bucket: &s.bucket})
	if e != nil {
		return ErrUnsafeBucket
	}
	if a.Owner == nil || aws.ToString(a.Owner.ID) == "" || len(a.Grants) == 0 {
		return ErrUnsafeBucket
	}
	for _, g := range a.Grants {
		if g.Grantee == nil || g.Grantee.Type != types.TypeCanonicalUser || aws.ToString(g.Grantee.ID) != aws.ToString(a.Owner.ID) || g.Grantee.URI != nil || g.Grantee.EmailAddress != nil {
			return ErrUnsafeBucket
		}
	}
	return nil
}

// Conservative permission analysis: every Allow must name concrete IAM AWS
// principals. Wildcards, NotPrincipal, service/federated/unknown principals and
// ambiguous forms fail closed even when conditions/explicit Deny might narrow
// them. This avoids attempting to emulate a provider-specific IAM evaluator.
// Safe statements are retained verbatim; unsafe policies are never rewritten.
func privateStatements(statements []any) error {
	for _, raw := range statements {
		st, ok := raw.(map[string]any)
		if !ok {
			return ErrUnsafeBucket
		}
		switch st["Effect"] {
		case "Deny":
		case "Allow":
			if _, ok := st["NotPrincipal"]; ok {
				return ErrUnsafeBucket
			}
			principal, ok := st["Principal"].(map[string]any)
			if !ok || len(principal) != 1 {
				return ErrUnsafeBucket
			}
			raw, ok := principal["AWS"]
			if !ok {
				return ErrUnsafeBucket
			}
			var values []any
			switch v := raw.(type) {
			case string:
				values = []any{v}
			case []any:
				values = v
			default:
				return ErrUnsafeBucket
			}
			if len(values) == 0 {
				return ErrUnsafeBucket
			}
			for _, v := range values {
				name, ok := v.(string)
				if !ok || !strings.HasPrefix(name, "arn:aws:iam::") || strings.ContainsAny(name, "*?") || !strings.Contains(name[13:], ":") {
					return ErrUnsafeBucket
				}
			}
		default:
			return ErrUnsafeBucket
		}
	}
	return nil
}
