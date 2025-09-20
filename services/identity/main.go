package main

import (
	"fmt"
	"os"

	"github.com/gabehamasaki/momentum/services/identity/database"
	_ "github.com/joho/godotenv/autoload"
)

func main() {
	database := database.NewDB(os.Getenv("IDENTITY_DSN"))

	fmt.Println("Connecting to database...")
	_, err := database.Conn()
	if err != nil {
		panic(err)
	}
	fmt.Println("Connected to database successfully.")

	fmt.Println("Running migrations...")
	err = database.Migrate()
	if err != nil {
		panic(err)
	}
	fmt.Println("Migrations run successfully.")

	fmt.Println("Seeding database...")
	err = database.Seeder()
	if err != nil {
		panic(err)
	}
	fmt.Println("Database seeded successfully.")
}
