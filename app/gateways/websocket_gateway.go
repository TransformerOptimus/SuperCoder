package gateways

import (
	"ai-developer/app/middleware"
	"ai-developer/app/services"
	"fmt"
	socketio "github.com/googollee/go-socket.io"
	"go.uber.org/zap"
	"strconv"
)

type WorkspaceGateway struct {
	projectService *services.ProjectService
	jwtAuth        *middleware.JWTClaims
	logger         *zap.Logger
}

func (w *WorkspaceGateway) OnConnect(s socketio.Conn) error {
	s.SetContext(make(map[string]interface{}))
	w.logger.Info("Connected websocket connection", zap.String("connection_id", s.ID()))
	return nil
}

func (w *WorkspaceGateway) OnDisconnect(s socketio.Conn, reason string) {
	w.logger.Info("Disconnected websocket connection", zap.String("connection_id", s.ID()), zap.String("reason", reason))
	ctx := s.Context().(map[string]interface{})
	w.logger.Info("Connection Context", zap.Any("context", ctx))
	if projectIDStr, ok := ctx["project_id"]; ok {
		w.logger.Info("Project ID found in context", zap.Any("project_id", projectIDStr))
		projectID, err := strconv.Atoi(fmt.Sprintf("%v", projectIDStr))
		if err != nil {
			w.logger.Error("Error converting project_id to int", zap.Error(err))
			s.Emit("error", "Invalid project_id value")
			return
		}
		err = w.projectService.DeleteProjectWorkspace(projectID)
		if err != nil {
			w.logger.Error("Error deleting project workspace", zap.Error(err))
			s.Emit("error", fmt.Sprintf("Error deleting project workspace: %v", err))
			return
		}
	} else {
		w.logger.Info("Project ID not found or invalid type")
	}
}

func (wg *WorkspaceGateway) OnWorkspaceStartEvent(s socketio.Conn, data map[string]interface{}) {
	wg.logger.Info("Received data for workspace-start", zap.Any("data", data))
	projectIDStr, ok := data["project_id"].(string)
	if !ok {
		wg.logger.Error("Invalid project_id type")
		s.Emit("error", "Invalid project_id type")
		return
	}

	projectID, err := strconv.Atoi(projectIDStr)
	if err != nil {
		wg.logger.Error("Error converting project_id to int", zap.Error(err))
		s.Emit("error", "Invalid project_id value")
		return
	}

	err = wg.projectService.CreateProjectWorkspace(projectID, "python")
	if err != nil {
		wg.logger.Error("Error creating project workspace", zap.Error(err))
		s.Emit("error", fmt.Sprintf("Error creating project workspace: %v", err))
		return
	}
	wg.logger.Info("Updating Connection Context with project_id", zap.Int("project_id", projectID))
	ctx := s.Context().(map[string]interface{})
	ctx["project_id"] = projectIDStr
	s.Emit("workspace-started", fmt.Sprintf("Workspace started for project: %v", projectID))
}

func (wg *WorkspaceGateway) OnWorkspaceDeleteEvent(s socketio.Conn, data map[string]interface{}) {
	wg.logger.Info("Received data for workspace-close", zap.Any("data", data))
	projectIDStr, ok := data["project_id"].(string)
	if !ok {
		wg.logger.Error("Invalid project_id type")
		s.Emit("error", "Invalid project_id type")
		return
	}

	projectID, err := strconv.Atoi(projectIDStr)
	if err != nil {
		wg.logger.Error("Error converting project_id to int", zap.Error(err))
		s.Emit("error", "Invalid project_id value")
		return
	}

	err2 := wg.projectService.DeleteProjectWorkspace(projectID)
	if err2 != nil {
		wg.logger.Error("Error deleting project workspace", zap.Error(err2))
		s.Emit("error", fmt.Sprintf("Error deleting project workspace: %v", err2))
		return
	}
	s.Emit("workspace-closed", fmt.Sprintf("Workspace closed for project: %v", projectID))
}

func NewWorkspaceGateway(
	projectService *services.ProjectService,
	jwtAuth *middleware.JWTClaims,
	logger *zap.Logger,
) *WorkspaceGateway {
	return &WorkspaceGateway{
		projectService: projectService,
		jwtAuth:        jwtAuth,
		logger:         logger.Named("WebsocketGateway"),
	}
}
