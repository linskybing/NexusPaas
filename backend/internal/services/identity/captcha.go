package identity

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func getCaptcha(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	answer := fmt.Sprintf("%04d", randomInt(10000))
	id := "CAP" + strings.ToUpper(randomHex(8))
	payload := map[string]any{
		"id":         id,
		"captcha_id": id,
		"image":      renderCaptchaImage(answer),
		"expires_at": time.Now().UTC().Add(5 * time.Minute).Format(time.RFC3339),
	}
	if !app.Config.Production {
		payload["answer"] = answer
	}
	stored := shared.CloneMap(payload)
	stored["answer_hash"] = platform.HashSecret(answer)
	delete(stored, "answer")
	if _, err := app.Store.Create(r.Context(), captchasResource, stored); err != nil {
		slog.Warn("captcha store create skipped", "captcha_id", id, "error", err)
	}
	return http.StatusOK, payload, nil
}

func renderCaptchaImage(answer string) string {
	const (
		width  = 132
		height = 44
	)
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	background := color.RGBA{R: 246, G: 248, B: 250, A: 255}
	ink := color.RGBA{R: 28, G: 35, B: 43, A: 255}
	noise := color.RGBA{R: 160, G: 172, B: 184, A: 255}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, background)
		}
	}
	for i := 0; i < 26; i++ {
		x := int(randomInt(width))
		y := int(randomInt(height))
		drawRect(img, x, y, 1+int(randomInt(3)), 1+int(randomInt(3)), noise)
	}
	for i := 0; i < 6; i++ {
		x := int(randomInt(width - 18))
		y := int(randomInt(height))
		drawRect(img, x, y, 18+int(randomInt(24)), 1, noise)
	}
	for i, digit := range answer {
		x := 12 + i*28 + int(randomInt(3)) - 1
		y := 7 + int(randomInt(3)) - 1
		drawDigit(img, x, y, digit, ink)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "data:image/png;base64,"
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func drawDigit(img *image.RGBA, x, y int, digit rune, c color.Color) {
	segments := map[rune][7]bool{
		'0': {true, true, true, false, true, true, true},
		'1': {false, false, true, false, false, true, false},
		'2': {true, false, true, true, true, false, true},
		'3': {true, false, true, true, false, true, true},
		'4': {false, true, true, true, false, true, false},
		'5': {true, true, false, true, false, true, true},
		'6': {true, true, false, true, true, true, true},
		'7': {true, false, true, false, false, true, false},
		'8': {true, true, true, true, true, true, true},
		'9': {true, true, true, true, false, true, true},
	}
	active := segments[digit]
	const (
		w = 18
		h = 28
		t = 4
	)
	if active[0] {
		drawRect(img, x+t, y, w-2*t, t, c)
	}
	if active[1] {
		drawRect(img, x, y+t, t, h/2-t, c)
	}
	if active[2] {
		drawRect(img, x+w-t, y+t, t, h/2-t, c)
	}
	if active[3] {
		drawRect(img, x+t, y+h/2-t/2, w-2*t, t, c)
	}
	if active[4] {
		drawRect(img, x, y+h/2+t/2, t, h/2-t, c)
	}
	if active[5] {
		drawRect(img, x+w-t, y+h/2+t/2, t, h/2-t, c)
	}
	if active[6] {
		drawRect(img, x+t, y+h-t, w-2*t, t, c)
	}
}

func drawRect(img *image.RGBA, x, y, w, h int, c color.Color) {
	bounds := img.Bounds()
	for yy := max(y, bounds.Min.Y); yy < min(y+h, bounds.Max.Y); yy++ {
		for xx := max(x, bounds.Min.X); xx < min(x+w, bounds.Max.X); xx++ {
			img.Set(xx, yy, c)
		}
	}
}

func verifyCaptcha(app *platform.App, r *http.Request, id, answer string) bool {
	record, ok := app.Store.Get(r.Context(), captchasResource, id)
	if !ok {
		return false
	}
	app.Store.Delete(r.Context(), captchasResource, id)
	if expiredAt(record.Data, time.Now().UTC()) {
		return false
	}
	return platform.VerifySecret(textValue(record.Data, "answer_hash"), strings.TrimSpace(answer))
}

func captchaRequired(app *platform.App, r *http.Request, username string) bool {
	record, ok := app.Store.Get(r.Context(), loginFailuresResource, loginFailureID(username, requestIP(r)))
	return ok && intValue(record.Data, "failures", 0) >= defaultLoginMaxFailed
}
