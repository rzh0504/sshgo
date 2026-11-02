package main

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"sshgo/internal/agent"
	"sshgo/internal/store"
	"sshgo/locales"
)

const (
	maxAgentTextBytes   = 64 << 10
	maxAgentTextsBytes  = 128 << 10
	maxAgentImageBytes  = 5 << 20
	maxAgentImagesBytes = 10 << 20
)

func (a *App) attachAgentFiles() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:   a.win,
		Title:    locales.AgentAttach(),
		Multiple: true,
	})
	if err != nil {
		a.agentNotice(err.Error())
		return
	}
	if len(paths) == 0 {
		return
	}
	var added []agent.Attachment
	problem := ""
	for _, path := range paths {
		attachment, msg := readAgentAttachment(path)
		if msg != "" {
			if problem == "" {
				problem = msg
			}
			continue
		}
		added = append(added, attachment)
	}
	a.update(func() {
		var textTotal, imageTotal int64
		for _, attachment := range a.agent.attachments {
			if attachment.Kind == agent.AttachmentImage {
				imageTotal += attachment.Size
			} else {
				textTotal += attachment.Size
			}
		}
		for _, attachment := range added {
			if attachment.Kind == agent.AttachmentImage {
				if imageTotal+attachment.Size > maxAgentImagesBytes {
					if problem == "" {
						problem = locales.AgentAttachmentImagesTotal()
					}
					a.deleteAgentAttachmentFile(attachment)
					continue
				}
				imageTotal += attachment.Size
			} else {
				if textTotal+attachment.Size > maxAgentTextsBytes {
					if problem == "" {
						problem = locales.AgentAttachmentTotal()
					}
					continue
				}
				textTotal += attachment.Size
			}
			a.agent.attachments = append(a.agent.attachments, attachment)
		}
		if problem != "" {
			a.toast = problem
		}
	})
}

func readAgentAttachment(path string) (agent.Attachment, string) {
	name := filepath.Base(path)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return agent.Attachment{}, locales.AgentAttachmentReadFailed(name)
	}
	file, err := os.Open(path)
	if err != nil {
		return agent.Attachment{}, locales.AgentAttachmentReadFailed(name)
	}
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	file.Close()
	mime := http.DetectContentType(head[:n])
	switch mime {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
		if info.Size() > maxAgentImageBytes {
			return agent.Attachment{}, locales.AgentAttachmentImageTooLarge(name)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return agent.Attachment{}, locales.AgentAttachmentReadFailed(name)
		}
		if len(b) > maxAgentImageBytes {
			return agent.Attachment{}, locales.AgentAttachmentImageTooLarge(name)
		}
		id, err := store.SaveAttachment(b, imageExt(mime))
		if err != nil {
			return agent.Attachment{}, locales.AgentAttachmentReadFailed(name)
		}
		storedPath, _ := store.AttachmentPath(id)
		data := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b)
		return agent.Attachment{
			ID:   id,
			Kind: agent.AttachmentImage,
			Name: name,
			MIME: mime,
			Size: int64(len(b)),
			Data: data,
			Path: storedPath,
		}, ""
	}
	if info.Size() > maxAgentTextBytes {
		return agent.Attachment{}, locales.AgentAttachmentTooLarge(name)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return agent.Attachment{}, locales.AgentAttachmentReadFailed(name)
	}
	if len(b) > maxAgentTextBytes {
		return agent.Attachment{}, locales.AgentAttachmentTooLarge(name)
	}
	if !utf8.Valid(b) || bytes.IndexByte(b, 0) >= 0 {
		return agent.Attachment{}, locales.AgentAttachmentUnsupported(name)
	}
	return agent.Attachment{
		Kind:    agent.AttachmentText,
		Name:    name,
		Size:    int64(len(b)),
		Content: string(b),
	}, ""
}

func (a *App) agentNotice(message string) {
	a.update(func() { a.toast = message })
}

func (a *App) deleteAgentAttachmentFile(attachment agent.Attachment) {
	if attachment.Kind != agent.AttachmentImage || attachment.ID == "" {
		return
	}
	_ = store.DeleteAttachment(attachment.ID)
	delete(a.agent.previews, attachment.ID)
}

func (a *App) hydrateAgentAttachments(msgs []*agent.Message) {
	for _, message := range msgs {
		for i := range message.Attachments {
			attachment := &message.Attachments[i]
			if attachment.Kind != agent.AttachmentImage || attachment.ID == "" {
				continue
			}
			if path, err := store.AttachmentPath(attachment.ID); err == nil {
				attachment.Path = path
			}
			if data, err := store.LoadAttachment(attachment.ID); err == nil {
				attachment.Data = "data:" + attachment.MIME + ";base64," +
					base64.StdEncoding.EncodeToString(data)
			}
		}
	}
}

func imageExt(mime string) string {
	switch mime {
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	}
	return ".jpg"
}

func attachmentCacheKey(attachment agent.Attachment) string {
	if attachment.ID != "" {
		return attachment.ID
	}
	return attachment.Name + "-" + strconv.FormatInt(attachment.Size, 10)
}

func (a *App) agentAttachmentPreview(attachment agent.Attachment) ui.ImageSource {
	if attachment.Kind != agent.AttachmentImage {
		return nil
	}
	key := attachmentCacheKey(attachment)
	if preview, ok := a.agent.previews[key]; ok {
		return preview
	}
	var data []byte
	if attachment.Path != "" {
		data, _ = os.ReadFile(attachment.Path)
	}
	if len(data) == 0 && attachment.Data != "" {
		if i := strings.IndexByte(attachment.Data, ','); i >= 0 {
			data, _ = base64.StdEncoding.DecodeString(attachment.Data[i+1:])
		}
	}
	if len(data) == 0 {
		return nil
	}
	preview, err := ui.DecodeImage(data)
	if err != nil {
		return nil
	}
	if a.agent.previews == nil {
		a.agent.previews = map[string]ui.ImageSource{}
	}
	a.agent.previews[key] = preview
	return preview
}

func (a *App) agentPendingAttachments(c *ui.Context, pal *palette, locked bool) {
	if len(a.agent.attachments) == 0 {
		return
	}
	ui.Row(c).FillWidth().Wrap().Gap(6).Children(func() {
		remove := -1
		for i, attachment := range a.agent.attachments {
			key := "agent-attachment-" + strconv.Itoa(i)
			chip := ui.Row(c.Key(key)).Height(28).Padding(0, 6, 0, 4).Radius(7).Gap(5).
				AlignItems(ui.Center).Background(pal.chip)
			chip.Children(func() {
				if attachment.Kind == agent.AttachmentImage {
					if preview := a.agentAttachmentPreview(attachment); preview != nil {
						ui.Image(c, preview).Size(24, 24).Fit(ui.Cover).Radius(4).Clip()
					} else {
						ui.Icon(c, attachIcon).Size(12, 12).TextColor(pal.textMuted).Shrink(0)
					}
				} else {
					ui.Icon(c, attachIcon).Size(11, 11).TextColor(pal.textMuted).Shrink(0)
				}
				ui.Text(c, attachment.Name).FontSize(11.5).TextColor(pal.text).SingleLine().
					Ellipsis("…").MaxWidth(160).Tooltip(attachment.Name)
				x := ui.ButtonBase(c.Key(key+"-x")).Size(16, 16).Radius(4).Center().
					Label(locales.AgentAttachmentRemove()).Tooltip(locales.AgentAttachmentRemove()).
					TextColor(pal.textFaint)
				if locked {
					x.Disabled(true)
				}
				if x.Hovered() {
					x.Background(pal.rowHover).TextColor(pal.text)
				}
				x.Children(func() { ui.Icon(c, closeIcon).Size(9, 9) })
				if x.Clicked() {
					remove = i
				}
			})
		}
		if remove >= 0 {
			a.deleteAgentAttachmentFile(a.agent.attachments[remove])
			a.agent.attachments = append(a.agent.attachments[:remove], a.agent.attachments[remove+1:]...)
			a.repaint()
		}
	})
}

func (a *App) agentMessageAttachments(c *ui.Context, pal *palette, attachments []agent.Attachment) {
	var images, texts []agent.Attachment
	for _, attachment := range attachments {
		if attachment.Kind == agent.AttachmentImage {
			images = append(images, attachment)
		} else {
			texts = append(texts, attachment)
		}
	}
	if len(images) > 0 {
		ui.Row(c).Wrap().Gap(6).Children(func() {
			for _, attachment := range images {
				preview := a.agentAttachmentPreview(attachment)
				if preview == nil {
					a.agentTextAttachmentChip(c, pal, attachment)
					continue
				}
				b := ui.ButtonBase(c.Key("agent-image-"+attachmentCacheKey(attachment))).
					Size(100, 100).Radius(8).Clip().Label(attachment.Name).
					Tooltip(locales.AgentAttachmentOpen()).Border(1, pal.text.Alpha(0.08))
				if b.Hovered() {
					b.Background(pal.rowHover)
				}
				b.Children(func() { ui.Image(c, preview).Size(100, 100).Fit(ui.Cover) })
				if b.Clicked() && attachment.Path != "" {
					mygo.Shell.OpenPath(attachment.Path)
				}
			}
		})
	}
	if len(texts) > 0 {
		ui.Row(c).Wrap().Gap(4).Children(func() {
			for _, attachment := range texts {
				a.agentTextAttachmentChip(c, pal, attachment)
			}
		})
	}
}

func (a *App) agentTextAttachmentChip(c *ui.Context, pal *palette, attachment agent.Attachment) {
	ui.Row(c).Height(22).Padding(0, 7).Radius(5).Gap(4).AlignItems(ui.Center).
		Background(pal.control).Border(1, pal.text.Alpha(0.08)).Children(func() {
		ui.Icon(c, attachIcon).Size(10, 10).TextColor(pal.textMuted)
		ui.Text(c, attachment.Name).FontSize(10.5).TextColor(pal.textMuted).SingleLine().
			Ellipsis("…").MaxWidth(200).Tooltip(attachment.Name)
	})
}
