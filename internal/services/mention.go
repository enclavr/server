package services

import (
	"encoding/json"
	"log"
	"regexp"
	"strings"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/google/uuid"
)

var mentionRegex = regexp.MustCompile(`@(\w+)`)

type MentionService struct {
	db *database.Database
}

func NewMentionService(db *database.Database) *MentionService {
	return &MentionService{db: db}
}

type ParsedMention struct {
	Username string
	UserID   uuid.UUID
	Type     models.MentionType
}

func (s *MentionService) ParseMentions(content string, roomID uuid.UUID, mentionedBy uuid.UUID) ([]models.MessageMention, error) {
	var mentions []models.MessageMention

	if strings.Contains(content, "@all") {
		roomMentions, err := s.createAllMention(roomID, mentionedBy, models.MentionTypeAll)
		if err != nil {
			return nil, err
		}
		mentions = append(mentions, roomMentions...)
	}

	if strings.Contains(content, "@here") {
		hereMentions, err := s.createHereMention(roomID, mentionedBy)
		if err != nil {
			return nil, err
		}
		mentions = append(mentions, hereMentions...)
	}

	matches := mentionRegex.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		username := match[1]
		if username == "all" || username == "here" {
			continue
		}
		if seen[username] {
			continue
		}
		seen[username] = true

		var user models.User
		if err := s.db.Where("LOWER(username) = LOWER(?)", username).First(&user).Error; err != nil {
			continue
		}

		var userRoom models.UserRoom
		if err := s.db.Where("user_id = ? AND room_id = ?", user.ID, roomID).First(&userRoom).Error; err != nil {
			continue
		}

		if user.ID == mentionedBy {
			continue
		}

		mentions = append(mentions, models.MessageMention{
			UserID:      user.ID,
			MentionedBy: mentionedBy,
			Type:        models.MentionTypeUser,
		})
	}

	return mentions, nil
}

func (s *MentionService) createAllMention(roomID uuid.UUID, mentionedBy uuid.UUID, mentionType models.MentionType) ([]models.MessageMention, error) {
	var userRooms []models.UserRoom
	if err := s.db.Where("room_id = ?", roomID).Find(&userRooms).Error; err != nil {
		return nil, err
	}

	var mentions []models.MessageMention
	for _, ur := range userRooms {
		if ur.UserID == mentionedBy {
			continue
		}
		mentions = append(mentions, models.MessageMention{
			RoomID:      roomID,
			UserID:      ur.UserID,
			MentionedBy: mentionedBy,
			Type:        mentionType,
		})
	}
	return mentions, nil
}

func (s *MentionService) createHereMention(roomID uuid.UUID, mentionedBy uuid.UUID) ([]models.MessageMention, error) {
	var presences []models.Presence
	if err := s.db.Where("room_id = ? AND status IN ?", roomID, []string{"online", "away"}).Find(&presences).Error; err != nil {
		return nil, err
	}

	var mentions []models.MessageMention
	seen := make(map[uuid.UUID]bool)
	for _, p := range presences {
		if p.UserID == mentionedBy || seen[p.UserID] {
			continue
		}
		seen[p.UserID] = true
		mentions = append(mentions, models.MessageMention{
			RoomID:      roomID,
			UserID:      p.UserID,
			MentionedBy: mentionedBy,
			Type:        models.MentionTypeHere,
		})
	}
	return mentions, nil
}

func (s *MentionService) SaveMentions(mentions []models.MessageMention, messageID uuid.UUID, roomID uuid.UUID) error {
	for i := range mentions {
		mentions[i].MessageID = messageID
		mentions[i].RoomID = roomID
	}

	if len(mentions) == 0 {
		return nil
	}

	return s.db.Create(&mentions).Error
}

func (s *MentionService) CreateMentionNotifications(mentions []models.MessageMention, actorName string, roomID uuid.UUID, messageID uuid.UUID) {
	for _, mention := range mentions {
		notificationType := models.NotificationTypeMention
		title := actorName + " mentioned you"
		body := actorName + " mentioned you in a message"

		if mention.Type == models.MentionTypeAll {
			title = actorName + " mentioned @all"
			body = actorName + " mentioned everyone in the room"
			notificationType = models.NotificationTypeRoomMention
		} else if mention.Type == models.MentionTypeHere {
			title = actorName + " mentioned @here"
			body = actorName + " mentioned online users in the room"
			notificationType = models.NotificationTypeRoomMention
		}

		dataJSON := ""
		data := map[string]interface{}{
			"message_id":   messageID.String(),
			"room_id":      roomID.String(),
			"mention_type": string(mention.Type),
		}
		if dataBytes, err := json.Marshal(data); err == nil {
			dataJSON = string(dataBytes)
		}

		notification := models.Notification{
			UserID:    mention.UserID,
			Type:      notificationType,
			Title:     title,
			Body:      body,
			RoomID:    &roomID,
			MessageID: &messageID,
			ActorID:   &mention.MentionedBy,
			ActorName: actorName,
			Data:      dataJSON,
		}

		if err := s.db.Create(&notification).Error; err != nil {
			log.Printf("Failed to create mention notification for user %s: %v", mention.UserID, err)
		}
	}
}

func (s *MentionService) GetMentionsForUser(userID uuid.UUID, limit int) ([]models.MessageMention, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var mentions []models.MessageMention
	err := s.db.Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Find(&mentions).Error
	return mentions, err
}

func (s *MentionService) GetMentionsForMessage(messageID uuid.UUID) ([]models.MessageMention, error) {
	var mentions []models.MessageMention
	err := s.db.Where("message_id = ?", messageID).Find(&mentions).Error
	return mentions, err
}
