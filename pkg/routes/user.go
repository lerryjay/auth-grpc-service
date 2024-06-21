package routes

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/lerryjay/auth-grpc-service/pkg/helpers"
	"github.com/lerryjay/auth-grpc-service/pkg/pb"
	models "github.com/lerryjay/auth-grpc-service/pkg/pb/model"

	"gorm.io/gorm"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (h *Handler) CreateUser(ctx context.Context, req *models.User) (*models.User, error) {
	var user models.UserORM
	//  Checks auth fields exist
	checkUser := h.DB.First(&user, "email = ? OR (username = ? AND username != '') OR telephone = ?", req.Email, req.Username, req.Telephone)
	if checkUser.Error == nil {
		log.Println("Tried creating user. User exists")
		return nil, status.Errorf(codes.AlreadyExists,
			"User exists")
	}

	req.Id = uuid.New().String()
	hashPassword, err := helpers.HashPassword(req.Password)
	if err != nil {
		log.Println(err)
		return nil, status.Errorf(codes.Internal,
			"Could not generate new user password hash")
	}

	req.Password = hashPassword
	userOrm, err := req.ToORM(ctx)
	if err != nil {
		log.Println(err)
		return nil, status.Errorf(codes.Internal,
			"Unable to create user. DB failed to insert %s", err)
	}

	query := h.DB.Create(&userOrm)
	if query.Error != nil {
		log.Println(query.Error)
		return nil, status.Errorf(codes.Internal,
			"Unable to create user. DB failed to insert")
	}

	return req, nil
}

func (h *Handler) GetUser(ctx context.Context, req *pb.GetUserRequest) (*models.User, error) {

	var user models.UserORM
	query := h.DB.First(&user, "id = ?", req.Id)
	if query.Error != nil {
		log.Println(query.Error)
		return nil, status.Errorf(codes.NotFound,
			"User not found")
	}
	var createdAt *timestamppb.Timestamp
	if user.CreatedAt != nil {
		createdAt = timestamppb.New(*user.CreatedAt)
	}
	var updatedAt *timestamppb.Timestamp
	if user.UpdatedAt != nil {
		updatedAt = timestamppb.New(*user.UpdatedAt)
	}
	userData := &models.User{
		Id:                 user.Id,
		Email:              user.Email,
		Firstname:          user.Firstname,
		Lastname:           user.Lastname,
		Role:               user.Role,
		ImageUrl:           user.ImageUrl,
		Username:           user.Username,
		Bio:                user.Bio,
		Telephone:          user.Telephone,
		VerificationStatus: models.VerificationStatus(user.VerificationStatus),
		CreatedAt:          createdAt,
		UpdatedAt:          updatedAt,
	}
	return userData, nil
}

func (h *Handler) ListUsers(ctx context.Context, req *pb.ListUsersRequest) (*pb.ListUsersResponse, error) {
	// Define a struct to hold the specific columns you want to retrieve
	type UserColumns struct {
		Id                 string
		Email              string
		Firstname          string
		Lastname           string
		Role               string
		ImageUrl           string
		Username           string
		Bio                string
		VerificationStatus models.VerificationStatus
		Telephone          string
	}

	var usersColumnsList []UserColumns
	var users []*models.User
	var totalCount int64
	var totalPages int64
	var nextPage int64

	// Set up pagination options
	if req.Limit == 0 {
		req.Limit = 10
	}
	if req.Page == 0 {
		req.Page = 1
	}

	offset := (req.Page - 1) * req.Limit
	query := h.DB.Model(&models.UserORM{})

	if req.Role != nil && *req.Role != "" {
		query = query.Where("role = ?", req.Role)
	}

	if req.Name != nil && *req.Name != "" {
		query = query.Where("firstname LIKE ? OR lastname LIKE ?", "%"+*req.Name+"%", "%"+*req.Name+"%")
	}

	if req.Email != nil && *req.Email != "" {
		query = query.Where("email LIKE ?", "%"+*req.Email+"%")
	}

	// Query total count of users
	if err := query.Count(&totalCount).Error; err != nil {
		log.Println(err)
		return nil, status.Errorf(codes.Internal, "Could not convert user %s", err)
	}

	// Calculate total pages
	totalPages = totalCount / int64(req.Limit)
	if totalCount%int64(req.Limit) != 0 {
		totalPages++
	}

	// Query users with specific columns
	query.Offset(int(offset)).Limit(int(req.Limit)).Select("id, email, firstname, lastname, role, image_url, username, bio, verification_status").Find(&usersColumnsList)
	if query.Error != nil {
		log.Println(query.Error)
		return nil, status.Errorf(codes.Internal, "Unable to find users ")
	}

	// Convert UserColumnsList to models.User objects
	for _, userColumns := range usersColumnsList {
		userData := &models.User{
			Id:                 userColumns.Id,
			Email:              userColumns.Email,
			Firstname:          userColumns.Firstname,
			Lastname:           userColumns.Lastname,
			Role:               userColumns.Role,
			ImageUrl:           userColumns.ImageUrl,
			Username:           userColumns.Username,
			Bio:                userColumns.Bio,
			VerificationStatus: models.VerificationStatus(userColumns.VerificationStatus),
			Telephone:          userColumns.Telephone,
		}

		users = append(users, userData)
	}

	if req.Page < int32(totalPages) {
		nextPage = int64(req.Page) + 1
	} else {
		nextPage = 1
	}

	return &pb.ListUsersResponse{
		Users: users,
		Meta: &models.Meta{
			Limit:       req.Limit,
			CurrentPage: req.Page,
			TotalCount:  int32(totalCount),
			TotalPages:  int32(totalPages),
			NextPage:    int32(nextPage),
		},
	}, nil
}

func (h *Handler) UpdateUser(ctx context.Context, req *models.User) (*models.User, error) {

	var user models.UserORM
	query := h.DB.First(&user, "id = ?", req.Id)
	if query.Error != nil {
		log.Println("Error fetching user ", req.Id, query.Error)
		return nil, status.Errorf(codes.NotFound,
			"User not found")
	}

	userData, err := req.ToORM(ctx)
	if err != nil {
		log.Println("Unable to convert to ORM ", req, err)
		return nil, status.Errorf(codes.NotFound,
			"User not found")
	}

	userData.Password = user.Password
	userData.CreatedAt = user.CreatedAt
	userData.UpdatedAt = user.UpdatedAt
	userData.Role = user.Role
	userData.Email = user.Email
	userData.VerificationStatus = user.VerificationStatus
	userData.ImageUrl = user.ImageUrl
	userData.Username = user.Username

	h.DB.Save(&userData)

	return req, nil
}

func (h *Handler) DeleteUser(ctx context.Context, req *pb.DeleteUserRequest) (*emptypb.Empty, error) {

	// TODO: Implement soft Delete
	query := h.DB.Where("id = ?", req.GetId()).Delete(&models.UserORM{})
	if query.Error != nil {
		log.Println(query.Error)
		return &emptypb.Empty{}, status.Errorf(codes.NotFound,
			"User not found!")
	}

	return &emptypb.Empty{}, nil

}

func (h *Handler) VerifyUser(ctx context.Context, req *pb.VerifyUserRequest) (*emptypb.Empty, error) {
	// Check if user with the provided ID already exists
	var existingUser models.UserORM
	query := h.DB.First(&existingUser, "id =?", req.UserId)
	if query.Error != nil {
		log.Println("User not found for verification:", query.Error)
		return nil, status.Errorf(codes.NotFound, "User not found for verification")
	}
	// Save the verification status as PROCESSING
	existingUser.VerificationStatus = int32(models.VerificationStatus_PROCESSING)
	updateQuery := h.DB.Save(&existingUser)
	if updateQuery.Error != nil {
		log.Println("Failed to update user verification status to PROCESSING:", updateQuery.Error)
		return nil, status.Errorf(codes.Internal, "Failed to update user verification status to PROCESSING")
	}

	// Perform additional verification logic based on IdType
	var verificationErr error
	// Perform additional verification logic based on IdType
	switch req.IdType {
	case models.IdType_DRIVERS_LICENCE:
		res, err := h.VerifyDL(ctx, &pb.VerifyDLRequest{
			IdNumber:  req.IdNumber,
			Firstname: req.Firstname,
			Lastname:  req.Lastname,
			// Other fields...
		})

		if err != nil {
			return nil, err
		}

		// Update the id_number of UserVerification instead of UserID
		h.UpdateUserIDNumber(ctx, &pb.UpdateIDNumberRequest{
			IdNumber: req.IdNumber,    // Assuming res.Id contains the verified ID number
			UserId:   existingUser.Id, // Assuming UserORM has an ID field
		})
		h.UpdateUserIDType(ctx, &pb.UpdateIDTypeRequest{
			IdType: 0,
			UserId: existingUser.Id,
		})
		h.UpdateUserVerificationNames(ctx, &pb.UpdateUserNamesRequest{
			FirstName: res.Applicant.Firstname,
			LastName:  res.Applicant.Lastname,
			UserId:    existingUser.Id,
		})
	case models.IdType_PASSPORT:
		res, err := h.VerifyPassport(ctx, &pb.VerifyPassportRequest{
			IdNumber:  req.IdNumber,
			Firstname: req.Firstname,
			Lastname:  req.Lastname,
			// Other fields...
		})

		if err != nil {
			return nil, err
		}
		// Update the id_number of UserVerification
		h.UpdateUserIDNumber(ctx, &pb.UpdateIDNumberRequest{
			IdNumber: req.IdNumber,    // Assuming res.Id contains the verified ID number
			UserId:   existingUser.Id, // Assuming UserORM has an ID field
		})
		h.UpdateUserIDType(ctx, &pb.UpdateIDTypeRequest{
			IdType: 1,
			UserId: existingUser.Id,
		})
		h.UpdateUserVerificationNames(ctx, &pb.UpdateUserNamesRequest{
			FirstName: res.Applicant.Firstname,
			LastName:  res.Applicant.Lastname,
			UserId:    existingUser.Id,
		})
	case models.IdType_IDENTITY_CARD:
		res, err := h.VerifyNIN(ctx, &pb.VerifyNINRequest{
			IdNumber:  req.IdNumber,
			Firstname: req.Firstname,
			Lastname:  req.Lastname,

			// Other fields...
		})

		if err != nil {
			return nil, err
		}

		// Update the id_number of UserVerification
		h.UpdateUserIDNumber(ctx, &pb.UpdateIDNumberRequest{
			IdNumber: req.IdNumber,    // Assuming res.Id contains the verified ID number
			UserId:   existingUser.Id, // Assuming UserORM has an ID field
		})
		h.UpdateUserIDType(ctx, &pb.UpdateIDTypeRequest{
			IdType: 2,
			UserId: existingUser.Id,
		})
		h.UpdateUserVerificationNames(ctx, &pb.UpdateUserNamesRequest{
			FirstName: res.Applicant.Firstname,
			LastName:  res.Applicant.Lastname,
			UserId:    existingUser.Id,
		})
	default:
		verificationErr = status.Errorf(codes.InvalidArgument, "Invalid ID type for verification")
	}

	// Check if verification failed
	if verificationErr != nil {
		log.Println("Verification failed:", verificationErr)

		// Update status to FAILED
		existingUser.VerificationStatus = int32(models.VerificationStatus_FAILED)
		failedUpdateQuery := h.DB.Save(&existingUser)
		if failedUpdateQuery.Error != nil {
			log.Println("Failed to update user verification status to FAILED:", failedUpdateQuery.Error)
			return nil, status.Errorf(codes.Internal, "Failed to update user verification status to FAILED")
		}

		return nil, verificationErr
	}

	// After successful verification, update the verification status to VERIFIED
	existingUser.VerificationStatus = int32(models.VerificationStatus_VERIFIED)
	finalUpdateQuery := h.DB.Save(&existingUser)
	if finalUpdateQuery.Error != nil {
		log.Println("Failed to update user verification status to VERIFIED:", finalUpdateQuery.Error)
		return nil, status.Errorf(codes.Internal, "Failed to update user verification status to VERIFIED")
	}

	return &emptypb.Empty{}, nil
}

func (h *Handler) UpdateUserIDImage(ctx context.Context, req *pb.UpdateIDImageRequest) (*pb.UpdateIDImageResponse, error) {
	// First, check if the user exists in UserORM
	var user models.UserORM
	query := h.DB.First(&user, "id = ?", req.UserId)
	if query.Error != nil {
		if errors.Is(query.Error, gorm.ErrRecordNotFound) {
			// User does not exist in UserORM, create a new row
			user.Id = req.UserId
			// Assuming other necessary fields are set here
			if err := h.DB.Create(&user).Error; err != nil {
				log.Println("Error creating user ", req.UserId, err)
				return nil, status.Errorf(codes.Internal, "Database error")
			}
		} else {
			log.Println("Error fetching user ", req.UserId, query.Error)
			return nil, status.Errorf(codes.Internal, "Database error")
		}
	}

	// Next, check if the user exists in UserVerificationORM
	var verification models.UserVerificationORM
	query = h.DB.First(&verification, "user_id = ?", req.UserId)
	if query.Error != nil {
		if errors.Is(query.Error, gorm.ErrRecordNotFound) {
			// User does not exist in UserVerificationORM, create a new row
			verification.UserId = &req.UserId
			verification.IdFilePath = req.IdImagePath
			if err := h.DB.Create(&verification).Error; err != nil {
				log.Println("Error creating user verification ", req.UserId, err)
				return nil, status.Errorf(codes.Internal, "Database error")
			}
		} else {
			log.Println("Error fetching user verification ", req.UserId, query.Error)
			return nil, status.Errorf(codes.Internal, "Database error")
		}
	} else {
		// User exists in UserVerificationORM, update the necessary fields
		if err := h.DB.Model(&verification).Where("user_id = ?", req.UserId).Updates(models.UserVerificationORM{
			IdFilePath: req.IdImagePath,
		}).Error; err != nil {
			log.Println("Error updating user verification ", req.UserId, err)
			return nil, status.Errorf(codes.Internal, "Database error")
		}
	}

	// Convert the updated UserORM model back to a Pb model
	updatedUser, err := user.ToPB(ctx)
	if err != nil {
		log.Println("Unable to convert UserORM to User model", err)
		return nil, status.Errorf(codes.Internal, "Unable to update user ID image")
	}

	// Create a response object and populate it with the necessary data
	response := &pb.UpdateIDImageResponse{
		User: &updatedUser, // Assuming pb.UpdateIDImageResponse has a User field
	}

	return response, nil
}

func (h *Handler) UpdateUserSelfie(ctx context.Context, req *pb.UpdateSelfieRequest) (*pb.UpdateSelfieResponse, error) {
	// First, check if the user exists in UserORM
	var Verification models.UserVerificationORM
	query := h.DB.First(&Verification, "user_id = ?", req.UserId)
	if query.Error != nil {
		if errors.Is(query.Error, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "User not found")
		}
		log.Println("Error fetching user ", req.UserId, query.Error)
		return nil, status.Errorf(codes.Internal, "Database error")
	}

	// Get the idnumber from the user record
	idnumber := Verification.IdNumber
	// Verify the selfie image
	verifyReq := &pb.VerifyIdentityImageRequest{
		//PhotoUrl:    req.SelfiePath,
		PhotoBase64: req.PhotoBase64, // If you have a base64 string, provide it here
		IdNumber:    idnumber,        // Assuming the ID type for the selfie
	}
	verifyResp, err := h.VerifyIDImage(ctx, verifyReq)
	if err != nil {
		log.Println("Selfie verification failed:", err)
		return nil, status.Errorf(codes.InvalidArgument, "Selfie verification failed: %v", err)
	}

	// Check if the verification status is "verified"
	if verifyResp.Status.Status != "verified" {
		return nil, status.Errorf(codes.InvalidArgument, "Selfie verification failed")
	}

	// First, check if the user exists in UserORM
	var user models.UserORM
	query = h.DB.First(&user, "id = ?", req.UserId)
	if query.Error != nil {
		if errors.Is(query.Error, gorm.ErrRecordNotFound) {
			// User does not exist in UserORM, create a new row
			// user.Id = req.UserId
			// // Assuming other necessary fields are set here
			// if err := h.DB.Create(&user).Error; err != nil {
			// 	log.Println("Error creating user ", req.UserId, err)
			// 	return nil, status.Errorf(codes.Internal, "Database error")
			// }
		} else {
			log.Println("Error fetching user ", req.UserId, query.Error)
			return nil, status.Errorf(codes.Internal, "Database error")
		}
	} else {
		// User exists, update the user details if necessary
		// Assuming user.IDImagePath or other fields might need updates
		user.VerificationStatus = int32(models.VerificationStatus_VERIFIED)
		if err := h.DB.Save(&user).Error; err != nil {
			log.Println("Error updating user ", req.UserId, err)
			return nil, status.Errorf(codes.Internal, "Database error")
		}
	}

	// Next, check if the user exists in UserVerificationORM
	var verification models.UserVerificationORM
	query = h.DB.First(&verification, "user_id = ?", req.UserId)
	if query.Error != nil {
		if errors.Is(query.Error, gorm.ErrRecordNotFound) {
			// User does not exist in UserVerificationORM, create a new row
			verification.UserId = &req.UserId
			verification.Selfie = req.SelfiePath
			if err := h.DB.Create(&verification).Error; err != nil {
				log.Println("Error creating user verification ", req.UserId, err)
				return nil, status.Errorf(codes.Internal, "Database error")
			}
		} else {
			log.Println("Error fetching user verification ", req.UserId, query.Error)
			return nil, status.Errorf(codes.Internal, "Database error")
		}
	} else {
		// User exists in UserVerificationORM, update the necessary fields
		if err := h.DB.Model(&verification).Where("user_id = ?", req.UserId).Updates(models.UserVerificationORM{
			Selfie: req.SelfiePath,
		}).Error; err != nil {
			log.Println("Error updating user verification ", req.UserId, err)
			return nil, status.Errorf(codes.Internal, "Database error")
		}
	}

	// Convert the updated UserORM model back to a Pb model
	updatedUser, err := user.ToPB(ctx)
	if err != nil {
		log.Println("Unable to convert UserORM to User model", err)
		return nil, status.Errorf(codes.Internal, "Unable to update user Selfie image")
	}

	// Create a response object and populate it with the necessary data
	response := &pb.UpdateSelfieResponse{
		User: &updatedUser, // Assuming pb.UpdateSelfieResponse has a User field
	}

	return response, nil
}

// UpdateUserIDNumber updates the user's ID number in the database after verification
func (h *Handler) UpdateUserIDNumber(ctx context.Context, req *pb.UpdateIDNumberRequest) (*pb.UpdateIDNumberResponse, error) {
	// If verification is successful, proceed to update the database
	var user models.UserORM
	query := h.DB.First(&user, "id = ?", req.UserId)
	if query.Error != nil {
		if errors.Is(query.Error, gorm.ErrRecordNotFound) {
			// User does not exist in UserORM, create a new row
			user.Id = req.UserId
			// Assuming other necessary fields are set here
			if err := h.DB.Create(&user).Error; err != nil {
				log.Println("Error creating user ", req.UserId, err)
				return nil, status.Errorf(codes.Internal, "Database error")
			}
		} else {
			log.Println("Error fetching user ", req.UserId, query.Error)
			return nil, status.Errorf(codes.Internal, "Database error")
		}
	}

	// Next, check if the user exists in UserVerificationORM
	var verificationORM models.UserVerificationORM
	query = h.DB.First(&verificationORM, "user_id = ?", req.UserId)
	if query.Error != nil {
		if errors.Is(query.Error, gorm.ErrRecordNotFound) {
			// User does not exist in UserVerificationORM, create a new row
			verificationORM.UserId = &req.UserId
			verificationORM.IdNumber = string(req.IdNumber)
			if err := h.DB.Create(&verificationORM).Error; err != nil {
				log.Println("Error creating user verification ", req.UserId, err)
				return nil, status.Errorf(codes.Internal, "Database error")
			}
		} else {
			log.Println("Error fetching user verification ", req.UserId, query.Error)
			return nil, status.Errorf(codes.Internal, "Database error")
		}
	} else {
		// User exists in UserVerificationORM, update the necessary fields
		if err := h.DB.Model(&verificationORM).Where("user_id = ?", req.UserId).Updates(models.UserVerificationORM{
			//IdNumber: strconv.Itoa(int(req.IdNumber)),
			IdNumber: req.IdNumber,
			//IdNumber: req.IdNumber,
		}).Error; err != nil {
			log.Println("Error updating user verification ", req.UserId, err)
			return nil, status.Errorf(codes.Internal, "Database error")
		}
	}

	// Convert the updated UserORM model back to a Pb model
	updatedUser, err := user.ToPB(ctx)
	if err != nil {
		log.Println("Unable to convert UserORM to User model", err)
		return nil, status.Errorf(codes.Internal, "Unable to update user ID number")
	}

	// Create a response object and populate it with the necessary data
	response := &pb.UpdateIDNumberResponse{
		User: &updatedUser, // Assuming pb.UpdateIDNumberResponse has a User field
	}

	return response, nil
}

func (h *Handler) UpdateUserIDType(ctx context.Context, req *pb.UpdateIDTypeRequest) (*pb.UpdateIDTypeResponse, error) {
	// Fetch the user from the UserORM table
	var user models.UserORM
	query := h.DB.First(&user, "id = ?", req.UserId)
	if query.Error != nil {
		if errors.Is(query.Error, gorm.ErrRecordNotFound) {
			// User does not exist in UserORM, return an error
			log.Println("User not found ", req.UserId)
			return nil, status.Errorf(codes.NotFound, "User not found")
		} else {
			log.Println("Error fetching user ", req.UserId, query.Error)
			return nil, status.Errorf(codes.Internal, "Database error")
		}
	}

	// Fetch the user's verification information from UserVerificationORM
	var verificationORM models.UserVerificationORM
	query = h.DB.First(&verificationORM, "user_id = ?", req.UserId)
	if query.Error != nil {
		if errors.Is(query.Error, gorm.ErrRecordNotFound) {
			// User does not exist in UserVerificationORM, create a new row
			// verificationORM.UserId = &req.UserId
			// verificationORM.IdType = int32(req.IdType)
			if err := h.DB.Create(&verificationORM).Error; err != nil {
				log.Println("Error creating user verification ", req.UserId, err)
				return nil, status.Errorf(codes.Internal, "Database error")
			}
		} else {
			log.Println("Error fetching user verification ", req.UserId, query.Error)
			return nil, status.Errorf(codes.Internal, "Database error")
		}
	} else {
		// User exists in UserVerificationORM, update the necessary fields
		if err := h.DB.Model(&verificationORM).Where("user_id = ?", req.UserId).Updates(models.UserVerificationORM{
			IdType: int32(req.IdType),
		}).Error; err != nil {
			log.Println("Error updating user verification ", req.UserId, err)
			return nil, status.Errorf(codes.Internal, "Database error")
		}
	}

	// Convert the updated UserORM model back to a Pb model
	updatedUser, err := user.ToPB(ctx)
	if err != nil {
		log.Println("Unable to convert UserORM to User model", err)
		return nil, status.Errorf(codes.Internal, "Unable to update user ID type")
	}

	// Create a response object and populate it with the necessary data
	response := &pb.UpdateIDTypeResponse{
		User: &updatedUser, // Assuming pb.UpdateIDTypeResponse has a User field
	}

	return response, nil
}
func (h *Handler) UpdateUserProfilePicture(ctx context.Context, req *pb.UpdateProfilePictureRequest) (*models.User, error) {
	var user models.UserORM
	query := h.DB.First(&user, "id = ?", req.UserId)
	if query.Error != nil {
		log.Println("Error fetching user ", req.UserId, query.Error)
		return nil, status.Errorf(codes.NotFound, "User not found")
	}

	user.ImageUrl = req.ProfilePicturePath
	h.DB.Save(&user)

	// Convert the ORM user back to the protobuf User model if needed
	updatedUser := &models.User{
		Id:       user.Id,
		Email:    user.Email,
		Role:     user.Role,
		ImageUrl: user.ImageUrl,
	}

	return updatedUser, nil
}

func (h *Handler) UpdateUserAddress(ctx context.Context, req *pb.UpdateUserAddressRequest) (*models.Address, error) {
	var address models.AddressORM
	query := h.DB.First(&address, "user_id = ?", req.UserId)
	now := time.Now()
	if errors.Is(query.Error, gorm.ErrRecordNotFound) {
		// If the address is not found, create a new address entry
		address = models.AddressORM{
			Street:      req.Street,
			City:        req.City,
			State:       req.State,
			PostalCode:  req.PostalCode,
			Country:     req.Country,
			Latitude:    req.Latitude,
			Longitude:   req.Longitude,
			StateCode:   req.StateCode,
			CountryCode: req.CountryCode,
			Currency:    req.Currency,
			CreatedAt:   &now,
			UpdatedAt:   &now,
			UserId:      &req.UserId,
		}

		if err := h.DB.Create(&address).Error; err != nil {
			log.Println("Error creating new address for user ", req.UserId, err)
			return nil, status.Errorf(codes.Internal, "Error creating new address")
		}
	} else if query.Error != nil {
		log.Println("Error fetching address for user ", req.UserId, query.Error)
		return nil, status.Errorf(codes.Internal, "Error fetching address")
	} else {
		// If the address exists, update the address fields
		address.Street = req.Street
		address.City = req.City
		address.State = req.State
		address.PostalCode = req.PostalCode
		address.Country = req.Country
		address.Latitude = req.Latitude
		address.Longitude = req.Longitude
		address.StateCode = req.StateCode
		address.CountryCode = req.CountryCode
		address.Currency = req.Currency
		address.UpdatedAt = &now

		// Save the updated address
		if err := h.DB.Save(&address).Error; err != nil {
			log.Println("Error updating address for user ", req.UserId, err)
			return nil, status.Errorf(codes.Internal, "Error updating address")
		}
	}

	// Convert the ORM address back to the protobuf Address model if needed
	updatedAddress := &models.AddressORM{
		Id:          address.Id,
		Street:      address.Street,
		City:        address.City,
		State:       address.State,
		PostalCode:  address.PostalCode,
		Country:     address.Country,
		Latitude:    address.Latitude,
		Longitude:   address.Longitude,
		StateCode:   address.StateCode,
		CountryCode: address.CountryCode,
		Currency:    address.Currency,
		// CreatedAt:   timestamppb.New(*address.CreatedAt),
		// UpdatedAt:   timestamppb.New(*address.UpdatedAt),
		UserId: &req.UserId,
	}
	addresses, _ := updatedAddress.ToPB(ctx)
	return &addresses, nil
}

func (h *Handler) UpdateUserVerificationNames(ctx context.Context, req *pb.UpdateUserNamesRequest) (*pb.UpdateUserNamesResponse, error) {
	// Check if the user exists in UserVerificationORM
	var verificationORM models.UserVerificationORM
	query := h.DB.First(&verificationORM, "user_id = ?", req.UserId)
	if query.Error != nil {
		if errors.Is(query.Error, gorm.ErrRecordNotFound) {
			// User does not exist in UserVerificationORM, create a new row
			verificationORM.UserId = &req.UserId
			verificationORM.FirstName = req.FirstName
			verificationORM.LastName = req.LastName
			if err := h.DB.Create(&verificationORM).Error; err != nil {
				log.Println("Error creating user verification ", req.UserId, err)
				return nil, status.Errorf(codes.Internal, "Database error")
			}
		} else {
			log.Println("Error fetching user verification ", req.UserId, query.Error)
			return nil, status.Errorf(codes.Internal, "Database error")
		}
	} else {
		// User exists in UserVerificationORM, update the necessary fields
		if err := h.DB.Model(&verificationORM).Where("user_id = ?", req.UserId).Updates(models.UserVerificationORM{
			FirstName: req.FirstName,
			LastName:  req.LastName,
		}).Error; err != nil {
			log.Println("Error updating user verification ", req.UserId, err)
			return nil, status.Errorf(codes.Internal, "Database error")
		}
	}

	// Convert the updated UserVerificationORM model back to a Pb model
	updatedVerification, err := verificationORM.ToPB(ctx)
	if err != nil {
		log.Println("Unable to convert UserVerificationORM to UserVerification model", err)
		return nil, status.Errorf(codes.Internal, "Unable to update user names")
	}

	// Create a response object and populate it with the necessary data
	response := &pb.UpdateUserNamesResponse{
		UserVerification: &updatedVerification, // Assuming pb.UpdateUserNamesResponse has a UserVerification field
	}

	return response, nil
}

func (h *Handler) GetUserAddress(ctx context.Context, req *pb.GetUserAddressRequest) (*pb.GetUserAddressResponse, error) {
	var address models.AddressORM
	query := h.DB.First(&address, "user_id = ?", req.UserId)

	if query.Error != nil {
		if errors.Is(query.Error, gorm.ErrRecordNotFound) {
			// Address not found for the user
			log.Println("Address not found for user ", req.UserId)
			return nil, status.Errorf(codes.NotFound, "Address not found")
		}
		// Other errors during query execution
		log.Println("Error fetching address for user ", req.UserId, query.Error)
		return nil, status.Errorf(codes.Internal, "Error fetching address")
	}

	// Convert the ORM address to the protobuf Address model
	addressPB := &models.Address{
		Street:      address.Street,
		City:        address.City,
		State:       address.State,
		PostalCode:  address.PostalCode,
		Country:     address.Country,
		Latitude:    address.Latitude,
		Longitude:   address.Longitude,
		StateCode:   address.StateCode,
		CountryCode: address.CountryCode,
		Currency:    address.Currency,
		CreatedAt:   timestamppb.New(*address.CreatedAt),
		UpdatedAt:   timestamppb.New(*address.UpdatedAt),
	}

	// Create a response object and populate it with the address data
	response := &pb.GetUserAddressResponse{
		Address: addressPB,
	}

	return response, nil
}
