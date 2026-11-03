package employees

import (
	"context"
	"github.com/haidarrasyad/absensi-api/internal/pkg/apperror"
	"github.com/haidarrasyad/absensi-api/internal/pkg/validation"
	"github.com/haidarrasyad/absensi-api/internal/shared"
)

func (s *Service) Save(ctx context.Context, id string, req WriteRequest) (Response, error) {
	if err := validation.Struct(&req); err != nil {
		return Response{}, err
	}
	if !shared.IsPublicID(req.MitraID, "MTR") {
		return Response{}, apperror.Validation(map[string]string{"mitra_id": "Mitra Tidak Valid"})
	}
	if !shared.IsPublicID(req.BidangID, "BID") {
		return Response{}, apperror.Validation(map[string]string{"bidang_id": "Bidang Tidak Valid"})
	}
	if id == "" && req.Phone == "" {
		return Response{}, apperror.Validation(map[string]string{"phone": "Nomor HP Wajib Diisi"})
	}
	if id != "" && !shared.IsPublicID(id, "EMP") {
		return Response{}, ErrNotFound
	}
	e := Employee{NIK: req.NIK, Name: req.Name, MitraID: req.MitraID, BidangID: req.BidangID}
	if req.Phone != "" {
		e.PhoneHash = s.hasher.Hash(req.Phone)
		var err error
		e.PhoneEncrypted, err = s.hasher.Encrypt(req.Phone)
		if err != nil {
			return Response{}, err
		}
	}
	result, err := s.repo.Save(ctx, id, e, req.IsActive)
	if err != nil {
		return Response{}, err
	}
	return s.adminResponse(result)
}
func (s *Service) Delete(ctx context.Context, id string) error {
	if !shared.IsPublicID(id, "EMP") {
		return ErrNotFound
	}
	return s.repo.Delete(ctx, id)
}

func (s *Service) adminResponse(e Employee) (Response, error) {
	result := ToResponse(e)
	phone, err := s.hasher.Decrypt(e.PhoneEncrypted, e.PhoneHash)
	if err != nil {
		return Response{}, err
	}
	result.Phone = phone
	result.NIK = e.NIK
	return result, nil
}
func (s *Service) AdminDetail(ctx context.Context, id string) (Response, error) {
	e, err := s.GetByID(ctx, id)
	if err != nil {
		return Response{}, err
	}
	return s.adminResponse(e)
}

func (s *Service) SetStatus(ctx context.Context, id string, active bool) (Response, error) {
	if !shared.IsPublicID(id, "EMP") {
		return Response{}, ErrNotFound
	}
	result, err := s.repo.SetStatus(ctx, id, active)
	if err != nil {
		return Response{}, err
	}
	return s.adminResponse(result)
}
