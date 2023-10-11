package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"reflect"
	"strconv"
	"time"

	swagger "github.com/gdg-garage/dungeons-and-trolls-go-client"
	"golang.org/x/exp/slices"
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
	cfg.BasePath = "http://10.0.1.63"

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
			log.Print(err)
			time.Sleep(time.Second)
			continue
		}
		// fmt.Println("Response:", resp)
		fmt.Println("Next tick ...")
		command := run(gameResp)
		logStruct(reflect.ValueOf(command), "Command")

		_, httpResp, err = client.DungeonsAndTrollsApi.DungeonsAndTrollsCommands(ctx, *command, nil)
		if err != nil {
			swaggerErr, ok := err.(swagger.GenericSwaggerError)
			if ok {
				log.Printf("Server error response: %s\n", swaggerErr.Body())
			} else {
				log.Printf("HTTP Response: %+v\n", httpResp)
				log.Print(err)
			}
			time.Sleep(time.Second)
			continue
		}
	}
}

func respawn(ctx context.Context, client *swagger.APIClient) {
	log.Println("Respawning ...")
	_, httpResp, err := client.DungeonsAndTrollsApi.DungeonsAndTrollsRespawn(ctx, struct{}{}, nil)
	if err != nil {
		log.Printf("HTTP Response: %+v\n", httpResp)
		log.Print(err)
	}
}

func logStruct(v reflect.Value, name string) {
	if v.Type().Kind() == reflect.Pointer {
		if !v.IsNil() {
			logStruct(v.Elem(), name)
		}
		return
	}

	if v.Type().Kind() == reflect.Struct {
		for n := 0; n < v.Type().NumField(); n++ {
			field := v.Field(n)
			fieldName := name + "." + v.Type().Field(n).Name
			logStruct(field, fieldName)
		}
		return
	}

	if v.Type().Kind() == reflect.Slice {
		for n := 0; n < v.Len(); n++ {
			field := v.Index(n)
			fieldName := name + "[" + strconv.Itoa(n) + "]"
			logStruct(field, fieldName)
		}
		return
	}

	log.Printf("%s: %v", name, v.Interface())
	return
}

func run(state swagger.DungeonsandtrollsGameState) *swagger.DungeonsandtrollsCommandsBatch {
	log.Println("Score:", state.Score)
	// logStruct(reflect.ValueOf(state.Character.Equip), "Character.Equip")
	log.Println()
	logStruct(reflect.ValueOf(state.Character.Attributes), "Character.Attributes")
	log.Println()
	log.Println("CurrentLevel:", state.CurrentLevel)
	log.Println("CurrentPosition.PositionX:", state.CurrentPosition.PositionX)
	log.Println("CurrentPosition.PositionY:", state.CurrentPosition.PositionY)

	if state.Character.SkillPoints > 1.5 {
		log.Println("Spending attribute points ...")
		return &swagger.DungeonsandtrollsCommandsBatch{
			AssignSkillPoints: spendAttributePoints(&state),
		}
	}

	var mainHandItem *swagger.DungeonsandtrollsItem
	for _, item := range state.Character.Equip {
		if *item.Slot == swagger.MAIN_HAND_DungeonsandtrollsItemType {
			mainHandItem = &item
			break
		}
	}

	if mainHandItem == nil && state.Character.Coordinates.Level == 0 {
		log.Println("Looking for items to buy ...")
		items := shop(&state)
		if len(items) > 0 {
			itemIds := make([]string, len(items))
			for i := range items {
				itemIds[i] = items[i].Id
			}

			return &swagger.DungeonsandtrollsCommandsBatch{
				Buy: &swagger.DungeonsandtrollsIdentifiers{Ids: itemIds},
			}
		} else {
			log.Println("ERROR: Found no item to buy!")
		}
	}

	stairsCoords := findStairs(&state)

	var skill *swagger.DungeonsandtrollsSkill

	if state.Character.Attributes.Stamina < 100 && state.Character.LastDamageTaken > 2 {
		for _, equip := range state.Character.Equip {
			for _, equipSkill := range equip.Skills {
				equipSkill := equipSkill

				if haveRequiredAttirbutes(state.Character.Attributes, equipSkill.Cost) &&
					equipSkill.CasterEffects != nil &&
					equipSkill.CasterEffects.Attributes != nil &&
					equipSkill.CasterEffects.Attributes.Stamina != nil &&
					calculateAttributesValue(state.Character.Attributes, equipSkill.CasterEffects.Attributes.Stamina) > 0 {

					skill = &equipSkill
					break
				}
			}
		}
		if skill != nil {
			log.Println("Resting")
			return &swagger.DungeonsandtrollsCommandsBatch{
				Skill: &swagger.DungeonsandtrollsSkillUse{
					SkillId: skill.Id,
				},
			}
		}
	}

	if state.Character.Attributes.Life < 100 && state.Character.LastDamageTaken > 2 {
		for _, equip := range state.Character.Equip {
			for _, equipSkill := range equip.Skills {
				equipSkill := equipSkill

				if haveRequiredAttirbutes(state.Character.Attributes, equipSkill.Cost) &&
					equipSkill.CasterEffects != nil &&
					equipSkill.CasterEffects.Attributes != nil &&
					equipSkill.CasterEffects.Attributes.Life != nil &&
					calculateAttributesValue(state.Character.Attributes, equipSkill.CasterEffects.Attributes.Life) > 0 {

					skill = &equipSkill
					break
				}
			}
		}
		if skill != nil {
			log.Println("Resting")
			return &swagger.DungeonsandtrollsCommandsBatch{
				Skill: &swagger.DungeonsandtrollsSkillUse{
					SkillId: skill.Id,
				},
			}
		}
	}

	monster := findMonster(&state)

	if monster != nil {
		maxDamage := float32(0)
		for _, equip := range state.Character.Equip {
			for _, equipSkill := range equip.Skills {
				equipSkill := equipSkill

				if haveRequiredAttirbutes(state.Character.Attributes, equipSkill.Cost) &&
					*equipSkill.Target == swagger.CHARACTER_SkillTarget &&
					equipSkill.DamageAmount != nil &&
					calculateAttributesValue(state.Character.Attributes, equipSkill.DamageAmount) > maxDamage {

					skill = &equipSkill
					maxDamage = calculateAttributesValue(state.Character.Attributes, equipSkill.DamageAmount)
				}
			}
		}

		if skill != nil {
			log.Println("Let's fight!")
			dist := distance(*state.CurrentPosition, *monster.Position)
			if dist <= int(calculateAttributesValue(state.Character.Attributes, skill.Range_)) {
				log.Println("Attacking ...")
				log.Println("Picked skill:", skill.Name, "with target type:", *skill.Target)
				damage := calculateAttributesValue(state.Character.Attributes, skill.DamageAmount)
				log.Println("Estimated damage ignoring resistances:", damage)

				if *skill.Target == swagger.POSITION_SkillTarget {
					return &swagger.DungeonsandtrollsCommandsBatch{
						Skill: &swagger.DungeonsandtrollsSkillUse{
							SkillId:  skill.Id,
							Position: monster.Position,
						},
					}
				}
				if *skill.Target == swagger.CHARACTER_SkillTarget {
					return &swagger.DungeonsandtrollsCommandsBatch{
						Skill: &swagger.DungeonsandtrollsSkillUse{
							SkillId:  skill.Id,
							TargetId: monster.Monsters[0].Id,
						},
					}
				}
				return &swagger.DungeonsandtrollsCommandsBatch{
					Skill: &swagger.DungeonsandtrollsSkillUse{
						SkillId: skill.Id,
					},
				}
			} else {
				return &swagger.DungeonsandtrollsCommandsBatch{
					Move: monster.Position,
				}
			}
		} else {
			log.Println("No skill. Moving towards stairs ...")
			return &swagger.DungeonsandtrollsCommandsBatch{
				Move: stairsCoords,
			}
		}
	}

	log.Println("No monsters. Let's find stairs ...")

	if stairsCoords == nil {
		log.Println("Can't find stairs")
		return &swagger.DungeonsandtrollsCommandsBatch{
			Yell: &swagger.DungeonsandtrollsMessage{
				Text: "Where are the stairs? I can't find them!",
			},
		}
	}

	log.Println("Moving towards stairs ...")
	return &swagger.DungeonsandtrollsCommandsBatch{
		Move: stairsCoords,
	}
}

func spendAttributePoints(state *swagger.DungeonsandtrollsGameState) *swagger.DungeonsandtrollsAttributes {
	state.Character.SkillPoints--
	return &swagger.DungeonsandtrollsAttributes{
		Strength:       state.Character.SkillPoints / 13,
		Dexterity:      state.Character.SkillPoints / 13,
		Intelligence:   state.Character.SkillPoints / 13,
		Willpower:      state.Character.SkillPoints / 13,
		Constitution:   state.Character.SkillPoints / 13,
		SlashResist:    state.Character.SkillPoints / 13,
		PierceResist:   state.Character.SkillPoints / 13,
		FireResist:     state.Character.SkillPoints / 13,
		PoisonResist:   state.Character.SkillPoints / 13,
		ElectricResist: state.Character.SkillPoints / 13,
		Life:           state.Character.SkillPoints / 13,
		Stamina:        state.Character.SkillPoints / 13,
		Mana:           state.Character.SkillPoints / 13,
	}
}

func shop(state *swagger.DungeonsandtrollsGameState) []swagger.DungeonsandtrollsItem {
	type shopItem struct {
		Value float32
		Item  swagger.DungeonsandtrollsItem
	}
	bestItems := []shopItem{}

	shop := state.ShopItems
	for _, item := range shop {
		if item.Price <= state.Character.Money && haveRequiredAttirbutes(state.Character.Attributes, item.Requirements) {
			bestItems = append(bestItems, shopItem{
				Value: calculateAttributesValue(state.Character.Attributes, item.Attributes) / float32(item.Price),
				Item:  item,
			})
		}
	}

	slices.SortFunc(bestItems, func(a, b shopItem) int {
		if b.Value > a.Value {
			return -1
		}
		if b.Value < a.Value {
			return 1
		}
		return 0
	})

	res := []swagger.DungeonsandtrollsItem{}

	money := state.Character.Money
	slots := map[swagger.DungeonsandtrollsItemType]bool{}
	for _, item := range bestItems {
		if item.Item.Price <= money && !slots[*item.Item.Slot] {
			res = append(res, item.Item)
			slots[*item.Item.Slot] = true
			money -= item.Item.Price
		}
	}

	return res
}

func findMonster(state *swagger.DungeonsandtrollsGameState) *swagger.DungeonsandtrollsMapObjects {
	level := state.CurrentLevel
	for _, map_ := range state.Map_.Levels {
		if map_.Level != level {
			continue
		}
		closestDist := math.MaxInt
		var closest *swagger.DungeonsandtrollsMapObjects
		for i := range map_.Objects {
			object := map_.Objects[i]
			if len(object.Monsters) > 0 {
				for _, monster := range object.Monsters {
					if distance(*state.CurrentPosition, *object.Position) < closestDist && monster.Name != "Chest" {
						log.Printf("Found monster on position: %+v\n", object.Position)
						closestDist = distance(*state.CurrentPosition, *object.Position)
						closest = &object
					}
				}
			}
		}
		return closest
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
		maxPortal := 0
		var portalPos swagger.DungeonsandtrollsPosition
		for i := range map_.Objects {
			object := map_.Objects[i]
			if object.Portal != nil && object.Portal.DestinationFloor > int32(maxPortal) {
				maxPortal = int(object.Portal.DestinationFloor)
				portalPos = *object.Position
			}
		}
		if maxPortal > 0 {
			log.Printf("Found portal on position: %+v\n", portalPos)
			return &portalPos
		}
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

func calculateAttributesValue(myAttrs *swagger.DungeonsandtrollsAttributes, attrs *swagger.DungeonsandtrollsAttributes) float32 {
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
	return value
}

func haveRequiredAttirbutes(myAttrs *swagger.DungeonsandtrollsAttributes, requirements *swagger.DungeonsandtrollsAttributes) bool {
	return myAttrs.Strength >= requirements.Strength &&
		myAttrs.Dexterity >= requirements.Dexterity &&
		myAttrs.Intelligence >= requirements.Intelligence &&
		myAttrs.Willpower >= requirements.Willpower &&
		myAttrs.Constitution >= requirements.Constitution &&
		myAttrs.SlashResist >= requirements.SlashResist &&
		myAttrs.PierceResist >= requirements.PierceResist &&
		myAttrs.FireResist >= requirements.FireResist &&
		myAttrs.PoisonResist >= requirements.PoisonResist &&
		myAttrs.ElectricResist >= requirements.ElectricResist &&
		myAttrs.Life >= requirements.Life &&
		myAttrs.Stamina >= requirements.Stamina &&
		myAttrs.Mana >= requirements.Mana
}

func abs(i int) int {
	if i < 0 {
		return -i
	}
	return i
}

func distance(a, b swagger.DungeonsandtrollsPosition) int {
	return abs(int(a.PositionX)-int(b.PositionX)) + abs(int(a.PositionY)-int(b.PositionY))
}
