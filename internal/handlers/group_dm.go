package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/enclavr/server/pkg/validator"
	"github.com/google/uuid"
)

type GroupDMHandler struct {
	db *database.Database
}

func NewGroupDMHandler(db *database.Database) *GroupDMHandler {
	return &GroupDMHandler{db: db}
}

type CreateGroupDMRequest struct {
	Name      string      `json:"name"`
	MemberIDs []uuid.UUID `json:"member_ids"`
}

type SendGroupDMRequest struct {
	GroupDMID uuid.UUID `json:"group_dm_id"`
	Content   string    `json:"content"`
}

type GroupDMResponse struct {
	ID        uuid.UUID             `json:"id"`
	Name      string                `json:"name"`
	CreatedBy uuid.UUID             `json:"created_by"`
	Members   []GroupMemberResponse `json:"members"`
	CreatedAt time.Time             `json:"created_at"`
	UpdatedAt time.Time             `json:"updated_at"`
}

type GroupMemberResponse struct {
	UserID   uuid.UUID `json:"user_id"`
	Username string    `json:"username"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

type GroupDMMessageResponse struct {
	ID            uuid.UUID  `json:"id"`
	GroupDMID     uuid.UUID  `json:"group_dm_id"`
	SenderID      uuid.UUID  `json:"sender_id"`
	Username      string     `json:"username"`
	Content       string     `json:"content"`
	IsEdited      bool       `json:"is_edited"`
	IsDeleted     bool       `json:"is_deleted"`
	ForwardedFrom *uuid.UUID `json:"forwarded_from,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func (h *GroupDMHandler) CreateGroupDM(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	if userID == uuid.Nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreateGroupDMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.MemberIDs) < 1 {
		http.Error(w, "At least one other member is required", http.StatusBadRequest)
		return
	}

	if len(req.MemberIDs) > 9 {
		http.Error(w, "Maximum 10 members allowed (including creator)", http.StatusBadRequest)
		return
	}

	allMembers := make(map[uuid.UUID]bool)
	allMembers[userID] = true
	for _, memberID := range req.MemberIDs {
		if memberID == userID {
			continue
		}
		if allMembers[memberID] {
			http.Error(w, "Duplicate member IDs", http.StatusBadRequest)
			return
		}
		var user models.User
		if err := h.db.First(&user, "id = ?", memberID).Error; err != nil {
			http.Error(w, "User not found: "+memberID.String(), http.StatusNotFound)
			return
		}
		allMembers[memberID] = true
	}

	name := req.Name
	if name == "" {
		name = ""
	}

	groupDM := models.GroupDM{
		Name:      name,
		CreatedBy: userID,
	}

	if err := h.db.Create(&groupDM).Error; err != nil {
		http.Error(w, "Failed to create group DM", http.StatusInternalServerError)
		return
	}

	members := []models.GroupDMMember{
		{
			GroupDMID: groupDM.ID,
			UserID:    userID,
			Role:      "owner",
		},
	}

	for _, memberID := range req.MemberIDs {
		if memberID == userID {
			continue
		}
		members = append(members, models.GroupDMMember{
			GroupDMID: groupDM.ID,
			UserID:    memberID,
			Role:      "member",
		})
	}

	if err := h.db.Create(&members).Error; err != nil {
		h.db.Delete(&groupDM)
		http.Error(w, "Failed to add members to group DM", http.StatusInternalServerError)
		return
	}

	h.sendGroupDMResponse(w, &groupDM, members, http.StatusCreated)
}

func (h *GroupDMHandler) GetGroupDMs(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	if userID == uuid.Nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var memberships []models.GroupDMMember
	if err := h.db.Where("user_id = ?", userID).Find(&memberships).Error; err != nil {
		http.Error(w, "Failed to fetch group DMs", http.StatusInternalServerError)
		return
	}

	groupDMIDs := make([]uuid.UUID, len(memberships))
	for i, m := range memberships {
		groupDMIDs[i] = m.GroupDMID
	}

	if len(groupDMIDs) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]GroupDMResponse{})
		return
	}

	var groupDMs []models.GroupDM
	if err := h.db.Where("id IN ?", groupDMIDs).Find(&groupDMs).Error; err != nil {
		http.Error(w, "Failed to fetch group DMs", http.StatusInternalServerError)
		return
	}

	var allMembers []models.GroupDMMember
	h.db.Where("group_dm_id IN ?", groupDMIDs).Find(&allMembers)

	memberUserIDs := make([]uuid.UUID, len(allMembers))
	for i, m := range allMembers {
		memberUserIDs[i] = m.UserID
	}

	var users []models.User
	userMap := make(map[uuid.UUID]models.User)
	if len(memberUserIDs) > 0 {
		h.db.Where("id IN ?", memberUserIDs).Find(&users)
		for _, u := range users {
			userMap[u.ID] = u
		}
	}

	membersByGroup := make(map[uuid.UUID][]GroupMemberResponse)
	for _, m := range allMembers {
		user := userMap[m.UserID]
		membersByGroup[m.GroupDMID] = append(membersByGroup[m.GroupDMID], GroupMemberResponse{
			UserID:   m.UserID,
			Username: user.Username,
			Role:     m.Role,
			JoinedAt: m.JoinedAt,
		})
	}

	var responses []GroupDMResponse
	for _, gdm := range groupDMs {
		responses = append(responses, GroupDMResponse{
			ID:        gdm.ID,
			Name:      gdm.Name,
			CreatedBy: gdm.CreatedBy,
			Members:   membersByGroup[gdm.ID],
			CreatedAt: gdm.CreatedAt,
			UpdatedAt: gdm.UpdatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responses)
}

func (h *GroupDMHandler) GetGroupDMMessages(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	if userID == uuid.Nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	groupDMIDStr := r.URL.Query().Get("group_dm_id")
	if groupDMIDStr == "" {
		http.Error(w, "group_dm_id is required", http.StatusBadRequest)
		return
	}

	groupDMID, err := uuid.Parse(groupDMIDStr)
	if err != nil {
		http.Error(w, "Invalid group_dm_id", http.StatusBadRequest)
		return
	}

	if !h.isMember(groupDMID, userID) {
		http.Error(w, "You are not a member of this group DM", http.StatusForbidden)
		return
	}

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	var messages []models.GroupDMMessage
	if err := h.db.Where("group_dm_id = ?", groupDMID).Order("created_at DESC").Limit(limit).Find(&messages).Error; err != nil {
		http.Error(w, "Failed to fetch messages", http.StatusInternalServerError)
		return
	}

	senderIDs := make([]uuid.UUID, len(messages))
	for i, msg := range messages {
		senderIDs[i] = msg.SenderID
	}

	senderMap := make(map[uuid.UUID]models.User)
	if len(senderIDs) > 0 {
		var senders []models.User
		h.db.Where("id IN ?", senderIDs).Find(&senders)
		for _, s := range senders {
			senderMap[s.ID] = s
		}
	}

	var responses []GroupDMMessageResponse
	for _, msg := range messages {
		content := msg.Content
		if msg.IsDeleted {
			content = ""
		}
		sender := senderMap[msg.SenderID]
		responses = append(responses, GroupDMMessageResponse{
			ID:            msg.ID,
			GroupDMID:     msg.GroupDMID,
			SenderID:      msg.SenderID,
			Username:      sender.Username,
			Content:       content,
			IsEdited:      msg.IsEdited,
			IsDeleted:     msg.IsDeleted,
			ForwardedFrom: msg.ForwardedFrom,
			CreatedAt:     msg.CreatedAt,
			UpdatedAt:     msg.UpdatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responses)
}

func (h *GroupDMHandler) SendGroupDMMessage(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	if userID == uuid.Nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req SendGroupDMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.GroupDMID == uuid.Nil {
		http.Error(w, "group_dm_id is required", http.StatusBadRequest)
		return
	}

	if req.Content == "" {
		http.Error(w, "Message content cannot be empty", http.StatusBadRequest)
		return
	}

	if err := validator.ValidateMessageContent(req.Content); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !h.isMember(req.GroupDMID, userID) {
		http.Error(w, "You are not a member of this group DM", http.StatusForbidden)
		return
	}

	msg := models.GroupDMMessage{
		GroupDMID: req.GroupDMID,
		SenderID:  userID,
		Content:   validator.SanitizeMessageContent(req.Content),
	}

	if err := h.db.Create(&msg).Error; err != nil {
		http.Error(w, "Failed to send message", http.StatusInternalServerError)
		return
	}

	var user models.User
	h.db.First(&user, "id = ?", userID)

	response := GroupDMMessageResponse{
		ID:        msg.ID,
		GroupDMID: msg.GroupDMID,
		SenderID:  msg.SenderID,
		Username:  user.Username,
		Content:   msg.Content,
		IsEdited:  msg.IsEdited,
		IsDeleted: msg.IsDeleted,
		CreatedAt: msg.CreatedAt,
		UpdatedAt: msg.UpdatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

func (h *GroupDMHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	if userID == uuid.Nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		GroupDMID uuid.UUID `json:"group_dm_id"`
		UserID    uuid.UUID `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if !h.isOwner(req.GroupDMID, userID) {
		http.Error(w, "Only the group owner can add members", http.StatusForbidden)
		return
	}

	var memberCount int64
	h.db.Model(&models.GroupDMMember{}).Where("group_dm_id = ?", req.GroupDMID).Count(&memberCount)
	if memberCount >= 10 {
		http.Error(w, "Maximum 10 members allowed", http.StatusBadRequest)
		return
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", req.UserID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if h.isMember(req.GroupDMID, req.UserID) {
		http.Error(w, "User is already a member", http.StatusConflict)
		return
	}

	member := models.GroupDMMember{
		GroupDMID: req.GroupDMID,
		UserID:    req.UserID,
		Role:      "member",
	}

	if err := h.db.Create(&member).Error; err != nil {
		http.Error(w, "Failed to add member", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "added"})
}

func (h *GroupDMHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	if userID == uuid.Nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		GroupDMID uuid.UUID `json:"group_dm_id"`
		UserID    uuid.UUID `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if !h.isOwner(req.GroupDMID, userID) && req.UserID != userID {
		http.Error(w, "Only the group owner can remove other members", http.StatusForbidden)
		return
	}

	if err := h.db.Where("group_dm_id = ? AND user_id = ?", req.GroupDMID, req.UserID).
		Delete(&models.GroupDMMember{}).Error; err != nil {
		http.Error(w, "Failed to remove member", http.StatusInternalServerError)
		return
	}

	var remainingMembers int64
	h.db.Model(&models.GroupDMMember{}).Where("group_dm_id = ?", req.GroupDMID).Count(&remainingMembers)
	if remainingMembers == 0 {
		h.db.Where("id = ?", req.GroupDMID).Delete(&models.GroupDM{})
		h.db.Where("group_dm_id = ?", req.GroupDMID).Delete(&models.GroupDMMessage{})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "removed"})
}

func (h *GroupDMHandler) LeaveGroupDM(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	if userID == uuid.Nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	groupDMIDStr := r.URL.Query().Get("group_dm_id")
	if groupDMIDStr == "" {
		http.Error(w, "group_dm_id is required", http.StatusBadRequest)
		return
	}

	groupDMID, err := uuid.Parse(groupDMIDStr)
	if err != nil {
		http.Error(w, "Invalid group_dm_id", http.StatusBadRequest)
		return
	}

	if !h.isMember(groupDMID, userID) {
		http.Error(w, "You are not a member of this group DM", http.StatusForbidden)
		return
	}

	h.db.Where("group_dm_id = ? AND user_id = ?", groupDMID, userID).Delete(&models.GroupDMMember{})

	var remainingMembers int64
	h.db.Model(&models.GroupDMMember{}).Where("group_dm_id = ?", groupDMID).Count(&remainingMembers)
	if remainingMembers == 0 {
		h.db.Where("id = ?", groupDMID).Delete(&models.GroupDM{})
		h.db.Where("group_dm_id = ?", groupDMID).Delete(&models.GroupDMMessage{})
	} else {
		var groupDM models.GroupDM
		if err := h.db.First(&groupDM, "id = ?", groupDMID).Error; err == nil {
			if groupDM.CreatedBy == userID {
				var newOwner models.GroupDMMember
				if err := h.db.Where("group_dm_id = ?", groupDMID).First(&newOwner).Error; err == nil {
					h.db.Model(&newOwner).Update("role", "owner")
					h.db.Model(&groupDM).Update("created_by", newOwner.UserID)
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "left"})
}

func (h *GroupDMHandler) isMember(groupDMID, userID uuid.UUID) bool {
	var count int64
	h.db.Model(&models.GroupDMMember{}).Where("group_dm_id = ? AND user_id = ?", groupDMID, userID).Count(&count)
	return count > 0
}

func (h *GroupDMHandler) isOwner(groupDMID, userID uuid.UUID) bool {
	var member models.GroupDMMember
	err := h.db.Where("group_dm_id = ? AND user_id = ? AND role = ?", groupDMID, userID, "owner").First(&member).Error
	return err == nil
}

func (h *GroupDMHandler) sendGroupDMResponse(w http.ResponseWriter, groupDM *models.GroupDM, members []models.GroupDMMember, status int) {
	userIDs := make([]uuid.UUID, len(members))
	for i, m := range members {
		userIDs[i] = m.UserID
	}

	var users []models.User
	userMap := make(map[uuid.UUID]models.User)
	if len(userIDs) > 0 {
		h.db.Where("id IN ?", userIDs).Find(&users)
		for _, u := range users {
			userMap[u.ID] = u
		}
	}

	var memberResponses []GroupMemberResponse
	for _, m := range members {
		user := userMap[m.UserID]
		memberResponses = append(memberResponses, GroupMemberResponse{
			UserID:   m.UserID,
			Username: user.Username,
			Role:     m.Role,
			JoinedAt: m.JoinedAt,
		})
	}

	response := GroupDMResponse{
		ID:        groupDM.ID,
		Name:      groupDM.Name,
		CreatedBy: groupDM.CreatedBy,
		Members:   memberResponses,
		CreatedAt: groupDM.CreatedAt,
		UpdatedAt: groupDM.UpdatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding group DM response: %v", err)
	}
}
