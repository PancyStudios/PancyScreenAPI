package discord

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/bwmarrin/discordgo"
)

var (
	session      *discordgo.Session
	channelLog   string
	channelError string
	channelTake  string
)

// Init loads the environment variables for Discord and starts the session
func Init() {
	token := os.Getenv("DISCORD_BOT_TOKEN")
	channelLog = os.Getenv("DISCORD_CHANNEL_LOG")
	channelError = os.Getenv("DISCORD_CHANNEL_ERROR")
	channelTake = os.Getenv("DISCORD_CHANNEL_TAKE")

	if token == "" {
		log.Println("DISCORD_BOT_TOKEN no configurado. Discord log deshabilitado.")
		return
	}

	var err error
	session, err = discordgo.New("Bot " + token)
	if err != nil {
		log.Printf("Error creando sesión de Discord: %v", err)
		return
	}

	// Connect to Gateway
	err = session.Open()
	if err != nil {
		log.Printf("Error abriendo conexión con Discord Gateway: %v", err)
		session = nil
		return
	}

	// Set presence
	_ = session.UpdateGameStatus(0, "Vigilando Capturas")
	log.Println("Bot de Discord conectado al Gateway exitosamente.")
}

func Close() {
	if session != nil {
		session.Close()
	}
}

func isConfigured() bool {
	return session != nil
}

// getUserAvatar fetches user info from Discord via discordgo
func getUserAvatar(userID string) string {
	defaultAvatar := "https://cdn.discordapp.com/embed/avatars/0.png"
	if userID == "" || !isConfigured() {
		return defaultAvatar
	}

	user, err := session.User(userID)
	if err != nil {
		return defaultAvatar
	}

	return user.AvatarURL("")
}

// SendTakeLog logs a new screenshot request
func SendTakeLog(targetURL string, userID string, isSFW bool) {
	if !isConfigured() || channelTake == "" {
		return
	}
	go func() {
		avatar := getUserAvatar(userID)
		mode := "NSFW"
		if isSFW {
			mode = "SFW"
		}

		embed := &discordgo.MessageEmbed{
			Title:       "📸 Nueva Petición de Captura",
			Description: fmt.Sprintf("**URL:** %s\n**Modo:** %s", targetURL, mode),
			Color:       0x00B0F4, // Blue
			Timestamp:   time.Now().Format(time.RFC3339),
			Footer: &discordgo.MessageEmbedFooter{
				Text:    fmt.Sprintf("User ID: %s", userID),
				IconURL: avatar,
			},
		}
		_, _ = session.ChannelMessageSendEmbed(channelTake, embed)
	}()
}

// SendErrorLog logs a screenshot error or security block
func SendErrorLog(targetURL string, userID string, errorMessage string, isSFW bool) {
	if !isConfigured() || channelError == "" {
		return
	}
	go func() {
		avatar := getUserAvatar(userID)
		mode := "NSFW"
		if isSFW {
			mode = "SFW"
		}

		embed := &discordgo.MessageEmbed{
			Title:       "❌ Error o Bloqueo de Seguridad",
			Description: fmt.Sprintf("**URL:** %s\n**Modo:** %s\n**Motivo:** %s", targetURL, mode, errorMessage),
			Color:       0xFF0000, // Red
			Timestamp:   time.Now().Format(time.RFC3339),
			Footer: &discordgo.MessageEmbedFooter{
				Text:    fmt.Sprintf("User ID: %s", userID),
				IconURL: avatar,
			},
		}
		_, _ = session.ChannelMessageSendEmbed(channelError, embed)
	}()
}

// SendSuccessLog uploads the image and sends a success log
func SendSuccessLog(targetURL string, userID string, isSFW bool, image []byte) {
	if !isConfigured() || channelLog == "" {
		return
	}
	go func() {
		avatar := getUserAvatar(userID)
		mode := "NSFW"
		if isSFW {
			mode = "SFW"
		}

		embed := &discordgo.MessageEmbed{
			Title:       "✅ Captura Generada Exitosamente",
			Description: fmt.Sprintf("**URL:** %s\n**Modo:** %s", targetURL, mode),
			Color:       0x00FF00, // Green
			Timestamp:   time.Now().Format(time.RFC3339),
			Footer: &discordgo.MessageEmbedFooter{
				Text:    fmt.Sprintf("User ID: %s", userID),
				IconURL: avatar,
			},
			Image: &discordgo.MessageEmbedImage{
				URL: "attachment://screenshot.png",
			},
		}

		file := &discordgo.File{
			Name:        "screenshot.png",
			ContentType: "image/png",
			Reader:      bytes.NewReader(image),
		}

		_, _ = session.ChannelMessageSendComplex(channelLog, &discordgo.MessageSend{
			Embeds: []*discordgo.MessageEmbed{embed},
			Files:  []*discordgo.File{file},
		})
	}()
}
