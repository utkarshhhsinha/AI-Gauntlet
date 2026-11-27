package handler

import (
	"crypto/rand"
	"embed"
	"math/big"
	"net/http"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

//go:embed preset_avatars/*.jpg
var presetAvatarsFS embed.FS

const presetAvatarURLPrefix = "/api/preset-avatars/"

var agentPresetAvatars = []string{
	"avatar-01.jpg",
	"avatar-02.jpg",
	"avatar-03.jpg",
	"avatar-04.jpg",
	"avatar-05.jpg",
	"avatar-06.jpg",
	"avatar-07.jpg",
	"avatar-08.jpg",
	"avatar-09.jpg",
	"avatar-10.jpg",
}

func randomAgentPresetAvatar() string {
	index, err := rand.Int(rand.Reader, big.NewInt(int64(len(agentPresetAvatars))))
	if err != nil {
		return presetAvatarURLPrefix + agentPresetAvatars[0]
	}
	return presetAvatarURLPrefix + agentPresetAvatars[index.Int64()]
}

// ServePresetAvatar — GET /api/preset-avatars/{name}
// Serves built-in profile avatars embedded from packages/ui/avatar.
func (h *Handler) ServePresetAvatar(w http.ResponseWriter, r *http.Request) {
	name := path.Base(chi.URLParam(r, "name"))
	if name == "" || name == "." || name == "/" || !strings.HasSuffix(name, ".jpg") {
		writeError(w, http.StatusNotFound, "avatar not found")
		return
	}
	data, err := presetAvatarsFS.ReadFile("preset_avatars/" + name)
	if err != nil {
		writeError(w, http.StatusNotFound, "avatar not found")
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// newAgentAvatar resolves the avatar to persist for a newly created agent. An
// explicit value is validated through acceptAvatarURL — a create path is still
// a way to publish a storage object, so it carries the same authorization
// boundary as an update. ok=false means the error response is already written
// and the caller must abort.
func (h *Handler) newAgentAvatar(w http.ResponseWriter, r *http.Request, avatarURL *string) (pgtype.Text, bool) {
	if avatarURL != nil && strings.TrimSpace(*avatarURL) != "" {
		accepted, ok := h.acceptAvatarURL(w, r, *avatarURL, "")
		if !ok {
			return pgtype.Text{}, false
		}
		return pgtype.Text{String: accepted, Valid: true}, true
	}
	return pgtype.Text{String: randomAgentPresetAvatar(), Valid: true}, true
}
