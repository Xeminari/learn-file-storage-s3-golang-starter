package main

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	const maxUploadSize = 1 << 30 // 1 GB
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}
	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't find video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Not authorized to update this video", nil)
		return
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type", err)
		return
	}
	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid file type", nil)
		return
	}
	if mediaType == "" {
		respondWithError(w, http.StatusBadRequest, "Missing Content-Type for thumbnail", nil)
		return
	}

	tmp, err := os.CreateTemp("", "tube-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create temp file", err)
		return
	}
	defer tmp.Close()
	defer os.Remove(tmp.Name())

	if _, err := io.Copy(tmp, file); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to save file", err)
		return
	}
	if _, err = tmp.Seek(0, io.SeekStart); err != nil {
		respondWithError(w, http.StatusInternalServerError, "could not seek temp file", err)
		return
	}

	aspectRatio, err := cfg.getVideoAspectRatio(tmp.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to get video aspect ratio", err)
		return
	}

	var prefix string
	switch aspectRatio {
	case "landscape":
		prefix = "landscape"
	case "portrait":
		prefix = "portrait"
	default:
		prefix = "other"
	}

	key := fmt.Sprintf("%s/%s", prefix, getAssetPath(mediaType))
	bucketparams := s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &key,
		Body:        tmp,
		ContentType: &mediaType,
	}
	_, err = cfg.s3Client.PutObject(r.Context(), &bucketparams)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to upload to S3", err)
		return
	}

	url := cfg.getObjectURL(key)
	video.VideoURL = &url
	if err := cfg.db.UpdateVideo(video); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
