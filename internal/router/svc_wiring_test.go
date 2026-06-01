package router

import (
	"github.com/go-playground/validator/v10"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

// Thin wrappers exposing service constructors with stable names for the test.
func newTemplateSvc(r dao.TemplateDAO, v *validator.Validate) service.TemplateService {
	return service.NewTemplateService(r, v)
}
func newLogSvc(r dao.LogDAO, tr dao.TemplateDAO, v *validator.Validate) service.LogService {
	return service.NewLogService(r, tr, v)
}
func newStatsSvc(r dao.StatsDAO, sr dao.SettingsDAO) service.StatsService {
	return service.NewStatsService(r, sr)
}
func newSettingsSvc(r dao.SettingsDAO, v *validator.Validate) service.SettingsService {
	return service.NewSettingsService(r, v)
}
func newProfileSvc(r dao.ProfileDAO, v *validator.Validate) service.ProfileService {
	return service.NewProfileService(r, v)
}
func newBodyWeightSvc(r dao.BodyWeightDAO, v *validator.Validate) service.BodyWeightService {
	return service.NewBodyWeightService(r, v)
}
