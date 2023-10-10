package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	swagger "github.com/gdg-garage/dungeons-and-trolls-go-client"
)

func main() {
	// Read command line arguments
	if len(os.Args) < 2 {
		log.Fatal("USAGE: ./dungeons-and-trolls-go-bot API_KEY")
	}
	apiKey := os.Args[1]

	// Initialize the HTTP client and set the base URL for the API
	cfg := swagger.NewConfiguration()
	// TODO: use prod path
	cfg.BasePath = "https://docker.tivvit.cz"

	// Set the X-API-key header value
	ctx := context.WithValue(context.Background(), swagger.ContextAPIKey, swagger.APIKey{Key: apiKey})

	// Create a new client instance
	client := swagger.NewAPIClient(cfg)

	if len(os.Args) > 2 && os.Args[2] == "respawn" {
		respawn(ctx, client)
		return
	}

	for {
		// Use the client to make API requests
		gameResp, httpResp, err := client.DungeonsAndTrollsApi.DungeonsAndTrollsGame(ctx, nil)
		if err != nil {
			log.Printf("HTTP Response: %+v\n", httpResp)
			log.Fatal(err)
		}
		// fmt.Println("Response:", resp)
		fmt.Println("Next tick ...")
		command := run(gameResp)
		fmt.Printf("Command: %+v\n", command)

		_, httpResp, err = client.DungeonsAndTrollsApi.DungeonsAndTrollsCommands(ctx, *command, nil)
		if err != nil {
			swaggerErr, ok := err.(swagger.GenericSwaggerError)
			if ok {
				log.Printf("Server error response: %s\n", swaggerErr.Body())
			} else {
				log.Printf("HTTP Response: %+v\n", httpResp)
				log.Fatal(err)
			}
		}
	}
}

func respawn(ctx context.Context, client *swagger.APIClient) {
	dummyPayload := ctx
	log.Println("Respawning ...")
	_, httpResp, err := client.DungeonsAndTrollsApi.DungeonsAndTrollsRespawn(ctx, dummyPayload, nil)
	if err != nil {
		log.Printf("HTTP Response: %+v\n", httpResp)
		log.Fatal(err)
	}
}

func run(state swagger.DungeonsandtrollsGameState) *swagger.DungeonsandtrollsCommandsBatch {
	log.Println("Score:", state.Score)
	log.Println("Character.Money:", state.Character.Money)
	log.Println("CurrentLevel:", state.CurrentLevel)
	log.Println("CurrentPosition.PositionX:", state.CurrentPosition.PositionX)
	log.Println("CurrentPosition.PositionY:", state.CurrentPosition.PositionY)

	if state.Character.SkillPoints > 0 {
		log.Println("Spending attribute points ...")
		return spendAttributePoints(&state)
	}

	var mainHandItem *swagger.DungeonsandtrollsItem
	for _, item := range state.Character.Equip {
		if *item.Slot == swagger.MAIN_HAND_DungeonsandtrollsItemType {
			mainHandItem = &item
			break
		}
	}

	if mainHandItem == nil {
		log.Println("Looking for items to buy ...")
		item := shop(&state)
		if item != nil {
			return &swagger.DungeonsandtrollsCommandsBatch{
				Buy: &swagger.DungeonsandtrollsIdentifiers{Ids: []string{item.Id}},
			}
		}
		log.Println("ERROR: Found no item to buy!")
	}

	if mainHandItem != nil {
		log.Println("I like this weapon:", mainHandItem.Name)

		monster := findMonster(&state)

		if monster != nil {
			log.Println("Let's fight!")
			if *monster.Position == *state.CurrentPosition && len(state.Character.Equip) > 0 {
				log.Println("Attacking ...")
				skill := state.Character.Equip[0].Skills[0]
				log.Println("Picked skill:", skill.Name, "with target type:", *skill.Target)
				damage := calculateAttributesValue(*state.Character.Attributes, *skill.DamageAmount)
				log.Println("Estimated damage ignoring resistances:", damage)

				if *skill.Target == swagger.POSITION_SkillTarget {
					return &swagger.DungeonsandtrollsCommandsBatch{
						Skill: &swagger.DungeonsandtrollsSkillUse{
							SkillId:  state.Character.Equip[0].Skills[0].Id,
							Position: monster.Position,
						},
					}
				}
				if *skill.Target == swagger.CHARACTER_SkillTarget {
					return &swagger.DungeonsandtrollsCommandsBatch{
						Skill: &swagger.DungeonsandtrollsSkillUse{
							SkillId:  state.Character.Equip[0].Skills[0].Id,
							TargetId: monster.Monsters[0].Id,
						},
					}
				}
				return &swagger.DungeonsandtrollsCommandsBatch{
					Skill: &swagger.DungeonsandtrollsSkillUse{
						SkillId: state.Character.Equip[0].Skills[0].Id,
					},
				}
			}

			return &swagger.DungeonsandtrollsCommandsBatch{
				Move: monster.Position,
			}
		}
	}
	log.Println("No monsters. Let's find stairs ...")

	stairsCoords := findStairs(&state)

	if stairsCoords == nil {
		log.Println("Can't find stairs")
		return &swagger.DungeonsandtrollsCommandsBatch{
			Yell: &swagger.DungeonsandtrollsMessage{
				Text: "Where are the stairs? I can't find them!",
			},
		}
	}

	if state.CurrentLevel > 7 {
		rand.Seed(time.Now().UnixNano())
		randomYell := rand.Intn(2)
		var yells []string = []string{
			"I'm so scared!",
			"Help me!",
		}
		return &swagger.DungeonsandtrollsCommandsBatch{
			Yell: &swagger.DungeonsandtrollsMessage{
				Text: yells[randomYell],
			},
		}
	}

	log.Println("Moving towards stairs ...")
	return &swagger.DungeonsandtrollsCommandsBatch{
		Move: stairsCoords,
	}
}

func spendAttributePoints(state *swagger.DungeonsandtrollsGameState) *swagger.DungeonsandtrollsCommandsBatch {
	return &swagger.DungeonsandtrollsCommandsBatch{
		AssignSkillPoints: &swagger.DungeonsandtrollsAttributes{
			Stamina: state.Character.SkillPoints,
		},
	}
}

func shop(state *swagger.DungeonsandtrollsGameState) *swagger.DungeonsandtrollsItem {
	shop := state.ShopItems
	for _, item := range shop {
		if item.Price <= state.Character.Money && *item.Slot == swagger.MAIN_HAND_DungeonsandtrollsItemType && item.Price == 0 {
			log.Println("Chosen item:", item.Name)
			return &item
		}
	}
	return nil
}

func findMonster(state *swagger.DungeonsandtrollsGameState) *swagger.DungeonsandtrollsMapObjects {
	level := state.CurrentLevel
	for _, map_ := range state.Map_.Levels {
		if map_.Level != level {
			continue
		}
		for i := range map_.Objects {
			object := map_.Objects[i]
			if len(object.Monsters) > 0 {
				log.Printf("Found monster on position: %+v\n", object.Position)
				return &object
			}
		}
	}
	return nil
}

func findStairs(state *swagger.DungeonsandtrollsGameState) *swagger.DungeonsandtrollsPosition {
	level := state.CurrentLevel
	log.Println("Current level:", level)
	for _, map_ := range state.Map_.Levels {
		if map_.Level != level {
			continue
		}
		log.Println("Found current level ...")
		for i := range map_.Objects {
			object := map_.Objects[i]
			if object.IsStairs {
				log.Printf("Found stairs on position: %+v\n", object.Position)
				return object.Position
			}
		}
	}
	return nil
}

func calculateAttributesValue(myAttrs swagger.DungeonsandtrollsAttributes, attrs swagger.DungeonsandtrollsAttributes) int {
	var value float32
	value += myAttrs.Strength * attrs.Strength
	value += myAttrs.Dexterity * attrs.Dexterity
	value += myAttrs.Intelligence * attrs.Intelligence
	value += myAttrs.Willpower * attrs.Willpower
	value += myAttrs.Constitution * attrs.Constitution
	value += myAttrs.SlashResist * attrs.SlashResist
	value += myAttrs.PierceResist * attrs.PierceResist
	value += myAttrs.FireResist * attrs.FireResist
	value += myAttrs.PoisonResist * attrs.PoisonResist
	value += myAttrs.ElectricResist * attrs.ElectricResist
	value += myAttrs.Life * attrs.Life
	value += myAttrs.Stamina * attrs.Stamina
	value += myAttrs.Mana * attrs.Mana
	value += attrs.Constant
	return int(value)
}
