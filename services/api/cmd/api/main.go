package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/as76513/JobSonar/services/api/internal/handlers"
	"github.com/as76513/JobSonar/services/api/internal/reviews"
	"github.com/as76513/JobSonar/services/api/internal/store"
)

func main() {
	dsn := env("POSTGRES_DSN", "postgres://jobsonar:jobsonar@localhost:5432/jobsonar?sslmode=disable")
	addr := env("API_ADDR", ":8080")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := store.New(ctx, dsn)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer db.Close()

	resumeDir, err := filepath.Abs(env("RESUME_DIR", "./data/resumes"))
	if err != nil {
		log.Fatalf("resume dir: %v", err)
	}
	if err := os.MkdirAll(resumeDir, 0o700); err != nil {
		log.Fatalf("resume dir: %v", err)
	}

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		BodyLimit:             6 << 20,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			if code >= 500 {
				log.Printf("api %s %s: %v", c.Method(), c.Path(), err)
			}
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
		},
	})
	app.Use(recover.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "http://localhost:5173,http://127.0.0.1:5173,http://localhost:3000",
		AllowMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))
	handlers.New(db, db, db, db, db, resumeDir).WithReviews(reviews.NewFromEnv(), db).WithTailor(db).Mount(app)

	go func() {
		log.Printf("api listening on %s", addr)
		if err := app.Listen(addr); err != nil {
			log.Fatalf("listen: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	_ = app.Shutdown()
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
