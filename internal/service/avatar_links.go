package service

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

const (
	// avatarLinkTTL is how long a link handed to a client keeps working.
	avatarLinkTTL = time.Hour
	// avatarLinkReuse is how long one link is handed out again. Reusing it
	// keeps the URL stable so the client's image cache works, and saves a
	// Storage round trip on most profile reads. It leaves at least
	// avatarLinkTTL-avatarLinkReuse of life on every link returned.
	avatarLinkReuse = 30 * time.Minute
	// avatarLinkSignTimeout bounds the Storage call so a slow Storage cannot
	// hold up the profile read that the app's launch waits on.
	avatarLinkSignTimeout = 3 * time.Second
	avatarLinkCacheMax    = 4096
)

type avatarLink struct {
	stored, signed string
	reuseUntil     time.Time
}

// avatarLinks hands out signed links for stored avatar references.
type avatarLinks struct {
	signer AvatarSigner
	now    func() time.Time

	mu    sync.Mutex
	cache map[uuid.UUID]avatarLink
}

func newAvatarLinks(signer AvatarSigner) *avatarLinks {
	return &avatarLinks{signer: signer, now: time.Now, cache: make(map[uuid.UUID]avatarLink)}
}

// present returns the profile as a client may see it. A reference into the
// private avatar bucket becomes a link that expires; any other https URL the
// athlete set is returned unchanged. The profile always comes back: a link that
// cannot be made is omitted, because a missing photo must not block sign-in.
func (l *avatarLinks) present(ctx context.Context, userID uuid.UUID, profile *model.UserProfile) *model.UserProfile {
	if l == nil || l.signer == nil || profile == nil || profile.AvatarURL == nil || *profile.AvatarURL == "" {
		return profile
	}
	stored := *profile.AvatarURL
	objectPath, ours := l.signer.ObjectPath(stored)
	if !ours {
		return profile
	}
	presented := *profile
	presented.AvatarURL = nil
	// Only this athlete's own bounded slots are ever signed.
	if !slices.Contains(avatarObjectPaths(userID), objectPath) {
		slog.WarnContext(ctx, "avatar reference is outside the athlete's own slots; omitted")
		return &presented
	}
	if signed, ok := l.cached(userID, stored); ok {
		presented.AvatarURL = &signed
		return &presented
	}
	signCtx, cancel := context.WithTimeout(ctx, avatarLinkSignTimeout)
	signed, err := l.signer.Sign(signCtx, objectPath, avatarLinkTTL)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "avatar link could not be signed; profile returned without a photo", "error", err)
		return &presented
	}
	l.store(userID, avatarLink{stored: stored, signed: signed, reuseUntil: l.now().Add(avatarLinkReuse)})
	presented.AvatarURL = &signed
	return &presented
}

// canonical maps a link a client sends back into the form the database keeps.
// It reports false for a reference to someone else's object.
func (l *avatarLinks) canonical(userID uuid.UUID, candidate string) (string, bool) {
	if l == nil || l.signer == nil {
		return candidate, true
	}
	objectPath, ours := l.signer.ObjectPath(candidate)
	if !ours {
		return candidate, true
	}
	if !slices.Contains(avatarObjectPaths(userID), objectPath) {
		return "", false
	}
	return l.signer.CanonicalURL(objectPath), true
}

func (l *avatarLinks) forget(userID uuid.UUID) {
	if l == nil {
		return
	}
	l.mu.Lock()
	delete(l.cache, userID)
	l.mu.Unlock()
}

func (l *avatarLinks) cached(userID uuid.UUID, stored string) (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	link, found := l.cache[userID]
	if !found || link.stored != stored || !l.now().Before(link.reuseUntil) {
		return "", false
	}
	return link.signed, true
}

func (l *avatarLinks) store(userID uuid.UUID, link avatarLink) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.cache) >= avatarLinkCacheMax {
		now := l.now()
		for id, existing := range l.cache {
			if !now.Before(existing.reuseUntil) {
				delete(l.cache, id)
			}
		}
		if len(l.cache) >= avatarLinkCacheMax {
			clear(l.cache)
		}
	}
	l.cache[userID] = link
}
