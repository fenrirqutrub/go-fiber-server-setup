package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	userCollection *mongo.Collection
	client         *mongo.Client
)

// User struct
type User struct {
	ID   primitive.ObjectID `json:"_id" bson:"_id,omitempty"`
	Name string             `json:"name" bson:"name"`
	Age  int                `json:"age" bson:"age"`
}

// Load .env
func loadEnv() {
	paths := []string{".env", "../.env", "../../.env"}
	for _, path := range paths {
		if err := godotenv.Load(path); err == nil {
			fmt.Printf("✅ .env loaded from: %s\n", path)
			break
		}
	}
}

// Connect MongoDB
func connectMongoDB() {
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		log.Fatal("❌ MONGO_URI missing! Create .env file with MONGO_URI=your_connection_string")
	}

	fmt.Println("🔄 Connecting to MongoDB...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var err error
	client, err = mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatal("❌ MongoDB connection failed:", err)
	}

	if err = client.Ping(ctx, nil); err != nil {
		log.Fatal("❌ MongoDB ping failed:", err)
	}

	userCollection = client.Database("fiberdb").Collection("users")
	fmt.Println("✅ MongoDB connected successfully!")
}

func main() {
	// Load environment
	loadEnv()

	// Connect MongoDB
	connectMongoDB()

	// Fiber app
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		},
	})

	// ===== ROUTES =====

	// Home
	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"message": "🚀 Fiber + MongoDB API running"})
	})

	// GET all users
	app.Get("/users", func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		cursor, err := userCollection.Find(ctx, bson.M{})
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch users"})
		}
		defer cursor.Close(ctx)

		var users []User
		if err := cursor.All(ctx, &users); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to decode users"})
		}

		if users == nil {
			users = []User{}
		}

		return c.JSON(users)
	})

	// POST create user
	app.Post("/user", func(c *fiber.Ctx) error {
		var user User
		if err := c.BodyParser(&user); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid JSON"})
		}
		if user.Name == "" {
			return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := userCollection.InsertOne(ctx, user)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to create user"})
		}

		return c.Status(201).JSON(fiber.Map{
			"message": "User created successfully",
			"id":      result.InsertedID,
			"user":    user,
		})
	})

	// PUT update user by name
	app.Put("/user/:name", func(c *fiber.Ctx) error {
		name := c.Params("name")
		var updateData User
		if err := c.BodyParser(&updateData); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid JSON"})
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		filter := bson.M{"name": name}
		update := bson.M{"$set": bson.M{"name": updateData.Name, "age": updateData.Age}}

		result, err := userCollection.UpdateOne(ctx, filter, update)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update user"})
		}

		if result.MatchedCount == 0 {
			return c.Status(404).JSON(fiber.Map{"error": "User not found"})
		}

		return c.JSON(fiber.Map{"message": "User updated successfully", "user": updateData})
	})

	// DELETE user by name
	app.Delete("/user/:name", func(c *fiber.Ctx) error {
		name := c.Params("name")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := userCollection.DeleteOne(ctx, bson.M{"name": name})
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to delete user"})
		}

		if result.DeletedCount == 0 {
			return c.Status(404).JSON(fiber.Map{"error": "User not found"})
		}

		return c.JSON(fiber.Map{"message": "User deleted successfully", "name": name})
	})

	// ===== SERVER START =====
	port := os.Getenv("PORT")
	if port == "" {
		port = "5000"
	}

	// Graceful shutdown
	go func() {
		if err := app.Listen(":" + port); err != nil {
			log.Fatal("❌ Server failed:", err)
		}
	}()
	fmt.Printf("🚀 Server running on http://localhost:%s\n", port)

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	fmt.Println("\n🛑 Shutting down server...")

	if err := app.Shutdown(); err != nil {
		log.Printf("❌ Fiber shutdown error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Disconnect(ctx); err != nil {
		log.Printf("❌ MongoDB disconnect error: %v", err)
	} else {
		fmt.Println("✅ MongoDB disconnected")
	}

	fmt.Println("✅ Server stopped gracefully")
}
