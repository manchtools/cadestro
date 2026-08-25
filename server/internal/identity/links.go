package identity

import (
	"context"

	"connectrpc.com/connect"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/store"
)

func (h *Handlers) ListIdentityLinks(ctx context.Context, req *connect.Request[cadestrov1.ListIdentityLinksRequest]) (*connect.Response[cadestrov1.ListIdentityLinksResponse], error) {
	actor, err := h.requireActor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, PermListIdentityLinks, actor.ID); err != nil {
		return nil, err
	}
	if !actor.CanOwnResources() {

		return connect.NewResponse(&cadestrov1.ListIdentityLinksResponse{}), nil
	}
	links, err := h.store.ListIdentityLinksForUser(ctx, actor.ID)
	if err != nil {
		return nil, internalError(ctx, "failed to list identity links")
	}
	resp := &cadestrov1.ListIdentityLinksResponse{}
	for _, l := range links {
		resp.Links = append(resp.Links, linkToProto(l))
	}
	return connect.NewResponse(resp), nil
}

func (h *Handlers) UnlinkIdentity(ctx context.Context, req *connect.Request[cadestrov1.UnlinkIdentityRequest]) (*connect.Response[cadestrov1.UnlinkIdentityResponse], error) {
	actor, err := h.requireActor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, PermUnlinkIdentity, actor.ID); err != nil {
		return nil, err
	}
	if !actor.CanOwnResources() {
		return nil, notFound(ctx, ErrIdentityLinkNotFound, "identity link not found")
	}

	link, err := h.store.GetIdentityLink(ctx, req.Msg.GetLinkId().GetValue())
	if err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, ErrIdentityLinkNotFound, "identity link not found")
		}
		return nil, internalError(ctx, "failed to load identity link")
	}
	if link.UserID != actor.ID {
		return nil, notFound(ctx, ErrIdentityLinkNotFound, "identity link not found")
	}

	existing, err := h.store.ListIdentityLinksForUser(ctx, actor.ID)
	if err != nil {
		return nil, internalError(ctx, "failed to load identity links")
	}
	if len(existing) <= 1 {
		return nil, rpcError(ctx, ErrLastAuthMethod, connect.CodeFailedPrecondition,
			"this is your only sign-in method; link another identity before removing it")
	}

	_, err = h.store.WithAudit(ctx, h.mutationOp(req, actor, PermUnlinkIdentity),
		func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
			removed, err := tx.DeleteIdentityLink(ctx, link.ID)
			if err != nil {
				return err
			}
			rec.Effect(store.AuditEffect{
				ResourceType:        "identity_link",
				ResourceID:          removed.ID,
				Action:              "UNLINK",
				Outcome:             store.EffectApplied,
				BeforeRef:           &removed.UserID,
				AfterRef:            &removed.ProviderID,
				EvidenceKind:        "external_subject_sha256",
				EvidenceFingerprint: fingerprint(removed.ExternalID),
			})
			return nil
		})
	if err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, ErrIdentityLinkNotFound, "identity link not found")
		}
		return nil, internalError(ctx, "failed to unlink identity")
	}
	return connect.NewResponse(&cadestrov1.UnlinkIdentityResponse{}), nil
}
