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

	lastYell := ""
	lastYellTick := int32(0)

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
		if command == nil {
			time.Sleep(time.Second)
			continue
		}

		if command.Yell != nil {
			if command.Yell.Text == lastYell && gameResp.Tick < lastYellTick+10 {
				command.Yell = nil
			} else {
				lastYell = command.Yell.Text
				lastYellTick = gameResp.Tick
			}
		}

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
	log.Println("Character.Money", state.Character.Money)
	// logStruct(reflect.ValueOf(state.Character.Equip), "Character.Equip")
	log.Println()
	logStruct(reflect.ValueOf(state.Character.Attributes), "Character.Attributes")
	log.Println()
	log.Println("CurrentLevel:", state.CurrentLevel)
	log.Println("CurrentPosition.PositionX:", state.CurrentPosition.PositionX)
	log.Println("CurrentPosition.PositionY:", state.CurrentPosition.PositionY)

	var mainHandItem *swagger.DungeonsandtrollsItem
	for _, item := range state.Character.Equip {
		if *item.Slot == swagger.MAIN_HAND_DungeonsandtrollsItemType {
			mainHandItem = &item
			break
		}
	}

	if state.Character.SkillPoints > 1.5 {
		log.Println("Spending attribute points ...")
		return &swagger.DungeonsandtrollsCommandsBatch{
			AssignSkillPoints: spendAttributePoints(&state),
			Yell: &swagger.DungeonsandtrollsMessage{
				Text: "Assigning skill points.",
			},
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
				Yell: &swagger.DungeonsandtrollsMessage{
					Text: "Buying swag.",
				},
			}
		} else {
			log.Println("ERROR: Found no item to buy!")
			return nil
		}
	}

	stairsCoords := findStairs(&state)
	monster := findMonster(&state)

	var attackSkill *swagger.DungeonsandtrollsSkill

	maxDamage := float32(0)
	for _, equip := range state.Character.Equip {
		for _, equipSkill := range equip.Skills {
			equipSkill := equipSkill

			if equipSkill.CasterEffects != nil && equipSkill.CasterEffects.Attributes != nil && equipSkill.CasterEffects.Attributes.Mana != nil && equipSkill.CasterEffects.Attributes.Mana.Mana < 0 {
				continue
			}

			if haveRequiredAttirbutes(state.Character.Attributes, equipSkill.Cost) &&
				*equipSkill.Target == swagger.CHARACTER_SkillTarget &&
				equipSkill.DamageAmount != nil &&
				calculateAttributesValue(state.Character.Attributes, equipSkill.DamageAmount) > maxDamage {

				damage := calculateAttributesValue(state.Character.Attributes, equipSkill.DamageAmount) * calculateAttributesValue(state.Character.Attributes, equipSkill.Range_)
				if damage > maxDamage {
					maxDamage = damage
					attackSkill = &equipSkill
				}
			}
		}
	}

	if (state.Character.Attributes.Stamina < state.Character.MaxAttributes.Stamina && state.Character.LastDamageTaken > 2 && (monster == nil || attackSkill == nil || distance(*state.CurrentPosition, *monster.Position) > int(calculateAttributesValue(state.Character.Attributes, attackSkill.Range_)+1))) ||
		(attackSkill != nil && !haveRequiredAttirbutes(state.Character.Attributes, attackSkill.Cost) && state.Character.LastDamageTaken > 2) ||
		(state.Character.Attributes.Mana < state.Character.MaxAttributes.Mana && state.Character.LastDamageTaken > 2 && (monster == nil || attackSkill == nil || distance(*state.CurrentPosition, *monster.Position) > int(calculateAttributesValue(state.Character.Attributes, attackSkill.Range_)+1))) ||
		(attackSkill != nil && !haveRequiredAttirbutes(state.Character.Attributes, attackSkill.Cost) && state.Character.LastDamageTaken > 2) {
		var skill *swagger.DungeonsandtrollsSkill

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
				Yell: &swagger.DungeonsandtrollsMessage{
					Text: "Resting.",
				},
			}
		}
	}

	if state.Character.Attributes.Life < 100 && state.Character.LastDamageTaken > 2 && (monster == nil || distance(*state.CurrentPosition, *monster.Position) > 3) {
		var skill *swagger.DungeonsandtrollsSkill

		for _, equip := range state.Character.Equip {
			for _, equipSkill := range equip.Skills {
				equipSkill := equipSkill

				if haveRequiredAttirbutes(state.Character.Attributes, equipSkill.Cost) &&
					equipSkill.TargetEffects != nil &&
					equipSkill.TargetEffects.Attributes != nil &&
					equipSkill.TargetEffects.Attributes.Life != nil &&
					calculateAttributesValue(state.Character.Attributes, equipSkill.TargetEffects.Attributes.Life) > 0 {

					skill = &equipSkill
					break
				}
			}
		}
		if skill != nil {
			log.Println("Healing")
			return &swagger.DungeonsandtrollsCommandsBatch{
				Skill: &swagger.DungeonsandtrollsSkillUse{
					SkillId:  skill.Id,
					TargetId: state.Character.Id,
				},
				Yell: &swagger.DungeonsandtrollsMessage{
					Text: "Healing.",
				},
			}
		}
	}

	if monster != nil {
		if attackSkill != nil {
			log.Println("Let's fight!")
			dist := mapDistance(*monster.Position, state)
			if dist <= int(calculateAttributesValue(state.Character.Attributes, attackSkill.Range_)) && lineOfSight(*monster.Position, state) {
				log.Println("Attacking ...")
				log.Println("Picked skill:", attackSkill.Name, "with target type:", *attackSkill.Target)
				damage := calculateAttributesValue(state.Character.Attributes, attackSkill.DamageAmount)
				log.Println("Estimated damage ignoring resistances:", damage)

				if *attackSkill.Target == swagger.POSITION_SkillTarget {
					return &swagger.DungeonsandtrollsCommandsBatch{
						Skill: &swagger.DungeonsandtrollsSkillUse{
							SkillId:  attackSkill.Id,
							Position: monster.Position,
						},
						Yell: &swagger.DungeonsandtrollsMessage{
							Text: fmt.Sprintf("%s!", attackSkill.Name),
						},
					}
				}
				if *attackSkill.Target == swagger.CHARACTER_SkillTarget {
					return &swagger.DungeonsandtrollsCommandsBatch{
						Skill: &swagger.DungeonsandtrollsSkillUse{
							SkillId:  attackSkill.Id,
							TargetId: monster.Monsters[0].Id,
						},
						Yell: &swagger.DungeonsandtrollsMessage{
							Text: fmt.Sprintf("%s!", attackSkill.Name),
						},
					}
				}
				return &swagger.DungeonsandtrollsCommandsBatch{
					Skill: &swagger.DungeonsandtrollsSkillUse{
						SkillId: attackSkill.Id,
					},
					Yell: &swagger.DungeonsandtrollsMessage{
						Text: fmt.Sprintf("%s!", attackSkill.Name),
					},
				}
			} else {
				return &swagger.DungeonsandtrollsCommandsBatch{
					Move: monster.Position,
					Yell: &swagger.DungeonsandtrollsMessage{
						Text: "Let's fight!",
					},
				}
			}
		} else {
			log.Println("No skill. Moving towards stairs ...")
			return &swagger.DungeonsandtrollsCommandsBatch{
				Move: findSpawn(&state),
				Yell: &swagger.DungeonsandtrollsMessage{
					Text: "Running away!",
				},
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

	if distance(*state.CurrentPosition, *stairsCoords) <= 1 && state.CurrentLevel != 0 {
		players := playersOnCurrentLevel(state)

		maxDist := 0
		var maxPlayer swagger.DungeonsandtrollsCharacter
		for _, player := range players {
			if player.Id == state.Character.Id {
				continue
			}

			dist := distance(*stairsCoords, coords2pos(*player.Coordinates))
			if dist > maxDist {
				maxDist = dist
				maxPlayer = player
			}
		}

		if maxDist > 1 {
			return &swagger.DungeonsandtrollsCommandsBatch{
				Yell: &swagger.DungeonsandtrollsMessage{
					Text: fmt.Sprintf("Hurry up, %s!", maxPlayer.Name),
				},
				Move: state.CurrentPosition,
			}
		}
	}

	log.Println("Moving towards stairs ...")
	return &swagger.DungeonsandtrollsCommandsBatch{
		Move: stairsCoords,
		Yell: &swagger.DungeonsandtrollsMessage{
			Text: "Going towards stairs.",
		},
	}
}

func spendAttributePoints(state *swagger.DungeonsandtrollsGameState) *swagger.DungeonsandtrollsAttributes {
	state.Character.SkillPoints -= 0.1
	return &swagger.DungeonsandtrollsAttributes{
		Strength:  state.Character.SkillPoints / 6,
		Dexterity: state.Character.SkillPoints / 6,
		// Intelligence: state.Character.SkillPoints / 7,
		// Willpower:    state.Character.SkillPoints / 7,
		Constitution: state.Character.SkillPoints / 6,
		SlashResist:  state.Character.SkillPoints / 6,
		PierceResist: state.Character.SkillPoints / 6,
		FireResist:   state.Character.SkillPoints / 6,
		// PoisonResist:   state.Character.SkillPoints / 13,
		// ElectricResist: state.Character.SkillPoints / 13,
		// Life:           state.Character.SkillPoints / 13,
		// Stamina:        state.Character.SkillPoints / 13,
		// Mana:           state.Character.SkillPoints / 13,
	}
}

func shop(state *swagger.DungeonsandtrollsGameState) []swagger.DungeonsandtrollsItem {
	type shopItem struct {
		Value float32
		Items []swagger.DungeonsandtrollsItem
	}

	shop := state.ShopItems

	var bestItems []shopItem
	newBestItems := []shopItem{}

	moneyLimit := float32(state.Character.Money) / 5
	restWeight := float32(0.05)
	cutoff := 500

	for _, item := range shop {
		if float32(item.Price) <= moneyLimit {
			maxDamage := float32(0)

			for _, skill := range item.Skills {
				if skill.DamageAmount != nil {
					if skill.CasterEffects != nil && skill.CasterEffects.Attributes != nil && skill.CasterEffects.Attributes.Mana != nil && skill.CasterEffects.Attributes.Mana.Mana < 0 {
						continue
					}

					damage := calculateAttributesValue(&swagger.DungeonsandtrollsAttributes{
						Strength:       1,
						Dexterity:      1,
						Intelligence:   1,
						Willpower:      1,
						Constitution:   1,
						SlashResist:    1,
						PierceResist:   1,
						FireResist:     1,
						PoisonResist:   1,
						ElectricResist: 1,
						Life:           1,
						Stamina:        1,
						Mana:           1,
					}, skill.DamageAmount)

					if damage > maxDamage {
						maxDamage = damage
					}
				}
			}

			if maxDamage > 0 {
				for _, item2 := range shop {
					attrs := addAttributes(
						state.Character.Attributes,
						item.Attributes,
						item2.Attributes,
					)

					if float32(item2.Price) <= moneyLimit &&
						haveRequiredAttirbutes(state.Character.Attributes, item.Requirements) &&
						haveRequiredAttirbutes(state.Character.Attributes, item2.Requirements) &&
						*item.Slot != *item2.Slot {

						skill, value := getItemDamage(&item, attrs)
						if skill != nil {
							value *= float32(math.Trunc(float64(calculateAttributesValue(attrs, skill.Range_))))
						}

						newBestItems = append(newBestItems, shopItem{
							Value: value,
							Items: []swagger.DungeonsandtrollsItem{
								item,
								item2,
							},
						})
					}
				}
			}
		}
	}

	log.Println("First pass: ", len(newBestItems))

	slices.SortFunc(newBestItems, func(a, b shopItem) int {
		if a.Value > b.Value {
			return -1
		}
		if a.Value < b.Value {
			return 1
		}
		return 0
	})
	bestItems = newBestItems[:min(len(newBestItems), cutoff)]
	newBestItems = newBestItems[:0]

	for _, bestItem := range bestItems {
		for _, item := range shop {
			if float32(item.Price) <= moneyLimit &&
				haveRequiredAttirbutes(state.Character.Attributes, item.Requirements) &&
				*item.Slot != *bestItem.Items[0].Slot &&
				*item.Slot != *bestItem.Items[1].Slot {
				attrs := addAttributes(
					state.Character.Attributes,
					bestItem.Items[0].Attributes,
					bestItem.Items[1].Attributes,
					item.Attributes,
				)

				_, aStam := getItemRest(&bestItem.Items[0], attrs)
				_, bStam := getItemRest(&bestItem.Items[1], attrs)
				_, cStam := getItemRest(&item, attrs)

				_, aPatch := getItemPatch(&bestItem.Items[0], attrs)
				_, bPatch := getItemPatch(&bestItem.Items[1], attrs)
				_, cPatch := getItemPatch(&item, attrs)

				if aStam > 0 || bStam > 0 || cStam > 0 {
					skill, value := getItemDamage(&bestItem.Items[0], attrs)
					value *= float32(math.Trunc(float64(calculateAttributesValue(attrs, skill.Range_))))
					value *= (1 + max(aStam, bStam, cStam)*restWeight)
					value *= (1 + max(aPatch, bPatch, cPatch)*restWeight)

					newBestItems = append(newBestItems, shopItem{
						Value: value,
						Items: append(bestItem.Items[0:2:2], item),
					})
				}
			}
		}
	}

	log.Println("Second pass: ", len(newBestItems))

	slices.SortFunc(newBestItems, func(a, b shopItem) int {
		if a.Value > b.Value {
			return -1
		}
		if a.Value < b.Value {
			return 1
		}
		return 0
	})
	bestItems = newBestItems[:min(len(newBestItems), cutoff)]
	newBestItems = newBestItems[:0]

	for _, bestItem := range bestItems {
		for _, item := range shop {
			if float32(item.Price) <= moneyLimit &&
				haveRequiredAttirbutes(state.Character.Attributes, item.Requirements) &&
				*item.Slot != *bestItem.Items[0].Slot &&
				*item.Slot != *bestItem.Items[1].Slot &&
				*item.Slot != *bestItem.Items[2].Slot {

				attrs := addAttributes(
					state.Character.Attributes,
					bestItem.Items[0].Attributes,
					bestItem.Items[1].Attributes,
					bestItem.Items[2].Attributes,
					item.Attributes,
				)

				_, aStam := getItemRest(&bestItem.Items[0], attrs)
				_, bStam := getItemRest(&bestItem.Items[1], attrs)
				_, cStam := getItemRest(&bestItem.Items[2], attrs)
				_, dStam := getItemRest(&item, attrs)

				_, aPatch := getItemPatch(&bestItem.Items[0], attrs)
				_, bPatch := getItemPatch(&bestItem.Items[1], attrs)
				_, cPatch := getItemPatch(&bestItem.Items[2], attrs)
				_, dPatch := getItemPatch(&item, attrs)

				if aPatch > 0 || bPatch > 0 || cPatch > 0 || dPatch > 0 {
					skill, value := getItemDamage(&bestItem.Items[0], attrs)
					value *= float32(math.Trunc(float64(calculateAttributesValue(attrs, skill.Range_))))
					value *= (1 + max(aStam, bStam, cStam, dStam)*restWeight)
					value *= (1 + max(aPatch, bPatch, cPatch, dPatch)*restWeight)
					value *= (1 + item.Attributes.SlashResist*0.2) * (1 + item.Attributes.PierceResist*1.5) * (1 + item.Attributes.FireResist*2)

					newBestItems = append(newBestItems, shopItem{
						Value: value,
						Items: append(bestItem.Items[0:3:3], item),
					})
				}
			}
		}
	}

	log.Println("Third pass: ", len(newBestItems))

	slices.SortFunc(newBestItems, func(a, b shopItem) int {
		if a.Value > b.Value {
			return -1
		}
		if a.Value < b.Value {
			return 1
		}
		return 0
	})
	bestItems = newBestItems[:min(len(newBestItems), cutoff)]
	newBestItems = newBestItems[:0]

	for _, bestItem := range bestItems {
		for _, item := range shop {
			if float32(item.Price) <= moneyLimit &&
				haveRequiredAttirbutes(state.Character.Attributes, item.Requirements) &&
				*item.Slot != *bestItem.Items[0].Slot &&
				*item.Slot != *bestItem.Items[1].Slot &&
				*item.Slot != *bestItem.Items[2].Slot &&
				*item.Slot != *bestItem.Items[3].Slot {

				attrs := addAttributes(
					state.Character.Attributes,
					bestItem.Items[0].Attributes,
					bestItem.Items[1].Attributes,
					bestItem.Items[2].Attributes,
					bestItem.Items[3].Attributes,
					item.Attributes,
				)

				_, aStam := getItemRest(&bestItem.Items[0], attrs)
				_, bStam := getItemRest(&bestItem.Items[1], attrs)
				_, cStam := getItemRest(&bestItem.Items[2], attrs)
				_, dStam := getItemRest(&bestItem.Items[3], attrs)
				_, eStam := getItemRest(&item, attrs)

				_, aPatch := getItemPatch(&bestItem.Items[0], attrs)
				_, bPatch := getItemPatch(&bestItem.Items[1], attrs)
				_, cPatch := getItemPatch(&bestItem.Items[2], attrs)
				_, dPatch := getItemPatch(&bestItem.Items[3], attrs)
				_, ePatch := getItemPatch(&item, attrs)

				skill, value := getItemDamage(&bestItem.Items[0], attrs)
				value *= float32(math.Trunc(float64(calculateAttributesValue(attrs, skill.Range_))))
				value *= (1 + max(aStam, bStam, cStam, dStam, eStam)*restWeight)
				value *= (1 + max(aPatch, bPatch, cPatch, dPatch, ePatch)*restWeight)
				value *= (1 + bestItem.Items[3].Attributes.SlashResist*0.2) * (1 + bestItem.Items[3].Attributes.PierceResist*1.5) * (1 + bestItem.Items[3].Attributes.FireResist*2)
				value *= (1 + item.Attributes.SlashResist*0.2) * (1 + item.Attributes.PierceResist*1.5) * (1 + item.Attributes.FireResist*2)

				newBestItems = append(newBestItems, shopItem{
					Value: value,
					Items: append(bestItem.Items[0:4:4], item),
				})
			}
		}
	}

	log.Println("Fourth pass: ", len(newBestItems))

	slices.SortFunc(newBestItems, func(a, b shopItem) int {
		if a.Value > b.Value {
			return -1
		}
		if a.Value < b.Value {
			return 1
		}
		return 0
	})
	bestItems = newBestItems[:min(len(newBestItems), cutoff)]
	newBestItems = newBestItems[:0]

	for _, bestItem := range bestItems {
		for _, item := range shop {
			if float32(item.Price) <= moneyLimit &&
				haveRequiredAttirbutes(state.Character.Attributes, item.Requirements) &&
				*item.Slot != *bestItem.Items[0].Slot &&
				*item.Slot != *bestItem.Items[1].Slot &&
				*item.Slot != *bestItem.Items[2].Slot &&
				*item.Slot != *bestItem.Items[3].Slot &&
				*item.Slot != *bestItem.Items[4].Slot {

				attrs := addAttributes(
					state.Character.Attributes,
					bestItem.Items[0].Attributes,
					bestItem.Items[1].Attributes,
					bestItem.Items[2].Attributes,
					bestItem.Items[3].Attributes,
					bestItem.Items[4].Attributes,
					item.Attributes,
				)

				_, aStam := getItemRest(&bestItem.Items[0], attrs)
				_, bStam := getItemRest(&bestItem.Items[1], attrs)
				_, cStam := getItemRest(&bestItem.Items[2], attrs)
				_, dStam := getItemRest(&bestItem.Items[3], attrs)
				_, eStam := getItemRest(&bestItem.Items[4], attrs)
				_, fStam := getItemRest(&item, attrs)

				_, aPatch := getItemPatch(&bestItem.Items[0], attrs)
				_, bPatch := getItemPatch(&bestItem.Items[1], attrs)
				_, cPatch := getItemPatch(&bestItem.Items[2], attrs)
				_, dPatch := getItemPatch(&bestItem.Items[3], attrs)
				_, ePatch := getItemPatch(&bestItem.Items[4], attrs)
				_, fPatch := getItemPatch(&item, attrs)

				skill, value := getItemDamage(&bestItem.Items[0], attrs)
				value *= float32(math.Trunc(float64(calculateAttributesValue(attrs, skill.Range_))))
				value *= (1 + max(aStam, bStam, cStam, dStam, eStam, fStam)*restWeight)
				value *= (1 + max(aPatch, bPatch, cPatch, dPatch, ePatch, fPatch)*restWeight)
				value *= (1 + bestItem.Items[3].Attributes.SlashResist*0.2) * (1 + bestItem.Items[3].Attributes.PierceResist*1.5) * (1 + bestItem.Items[3].Attributes.FireResist*2)
				value *= (1 + bestItem.Items[4].Attributes.SlashResist*0.2) * (1 + bestItem.Items[4].Attributes.PierceResist*1.5) * (1 + bestItem.Items[4].Attributes.FireResist*2)

				newBestItems = append(newBestItems, shopItem{
					Value: value,
					Items: append(bestItem.Items[0:5:5], item),
				})
			}
		}
	}

	log.Println("Fifth pass: ", len(newBestItems))

	slices.SortFunc(newBestItems, func(a, b shopItem) int {
		if a.Value > b.Value {
			return -1
		}
		if a.Value < b.Value {
			return 1
		}
		return 0
	})
	bestItems = newBestItems
	// newBestItems = newBestItems[:0]

	for _, setup := range bestItems {
		totalPrice := 0
		for _, item := range setup.Items {
			totalPrice += int(item.Price)
		}
		if totalPrice > int(state.Character.Money) {
			continue
		}

		return setup.Items
	}

	return nil
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
					if mapDistance(*object.Position, *state) < closestDist && monster.Name != "Chest" {
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
	for _, map_ := range state.Map_.Levels {
		if map_.Level != level {
			continue
		}
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

func findSpawn(state *swagger.DungeonsandtrollsGameState) *swagger.DungeonsandtrollsPosition {
	for _, map_ := range state.Map_.Levels {
		if map_.Level != state.CurrentLevel {
			continue
		}

		for i := range map_.Objects {
			object := map_.Objects[i]
			if object.IsSpawn {
				log.Printf("Found spawn on position: %+v\n", object.Position)
				return object.Position
			}
		}
	}
	return nil
}

func getItemDamage(item *swagger.DungeonsandtrollsItem, attrs *swagger.DungeonsandtrollsAttributes) (*swagger.DungeonsandtrollsSkill, float32) {
	var best *swagger.DungeonsandtrollsSkill
	bestValue := float32(0)

	for _, skill := range item.Skills {
		skill := skill
		if skill.CasterEffects != nil && skill.CasterEffects.Attributes != nil && skill.CasterEffects.Attributes.Mana != nil && skill.CasterEffects.Attributes.Mana.Mana < 0 {
			continue
		}

		if *skill.Target == swagger.CHARACTER_SkillTarget && skill.DamageAmount != nil {
			value := calculateAttributesValue(skill.DamageAmount, attrs)
			if value > bestValue {
				bestValue = value
				best = &skill
			}
		}
	}

	return best, bestValue
}

func getItemRest(item *swagger.DungeonsandtrollsItem, attrs *swagger.DungeonsandtrollsAttributes) (*swagger.DungeonsandtrollsSkill, float32) {
	var best *swagger.DungeonsandtrollsSkill
	bestValue := float32(0)

	for _, skill := range item.Skills {
		skill := skill

		if skill.CasterEffects != nil && skill.CasterEffects.Attributes != nil && skill.CasterEffects.Attributes.Stamina != nil && skill.CasterEffects.Attributes.Mana != nil {
			value := calculateAttributesValue(skill.CasterEffects.Attributes.Stamina, attrs) * calculateAttributesValue(skill.CasterEffects.Attributes.Mana, attrs)
			if value > bestValue {
				bestValue = value
				best = &skill
			}
		}
	}

	return best, bestValue
}

func getItemPatch(item *swagger.DungeonsandtrollsItem, attrs *swagger.DungeonsandtrollsAttributes) (*swagger.DungeonsandtrollsSkill, float32) {
	var best *swagger.DungeonsandtrollsSkill
	bestValue := float32(0)

	for _, skill := range item.Skills {
		skill := skill

		if skill.TargetEffects != nil && skill.TargetEffects.Attributes != nil && skill.TargetEffects.Attributes.Life != nil {
			value := calculateAttributesValue(skill.TargetEffects.Attributes.Life, attrs)
			if value > bestValue {
				bestValue = value
				best = &skill
			}
		}
	}

	return best, bestValue
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

func addAttributes(attrs ...*swagger.DungeonsandtrollsAttributes) *swagger.DungeonsandtrollsAttributes {
	if len(attrs) == 0 {
		return nil
	}
	if len(attrs) == 1 {
		return attrs[0]
	}

	firstAttrs := attrs[0]
	otherAttrs := addAttributes(attrs[1:]...)

	return &swagger.DungeonsandtrollsAttributes{
		Strength:       firstAttrs.Strength + otherAttrs.Strength,
		Dexterity:      firstAttrs.Dexterity + otherAttrs.Dexterity,
		Intelligence:   firstAttrs.Intelligence + otherAttrs.Intelligence,
		Willpower:      firstAttrs.Willpower + otherAttrs.Willpower,
		Constitution:   firstAttrs.Constitution + otherAttrs.Constitution,
		SlashResist:    firstAttrs.SlashResist + otherAttrs.SlashResist,
		PierceResist:   firstAttrs.PierceResist + otherAttrs.PierceResist,
		FireResist:     firstAttrs.FireResist + otherAttrs.FireResist,
		PoisonResist:   firstAttrs.PoisonResist + otherAttrs.PoisonResist,
		ElectricResist: firstAttrs.ElectricResist + otherAttrs.ElectricResist,
		Life:           firstAttrs.Life + otherAttrs.Life,
		Stamina:        firstAttrs.Stamina + otherAttrs.Stamina,
		Mana:           firstAttrs.Mana + otherAttrs.Mana,
		Constant:       firstAttrs.Constant + otherAttrs.Constant,
	}
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

func lineOfSight(position swagger.DungeonsandtrollsPosition, state swagger.DungeonsandtrollsGameState) bool {
	for _, level := range state.Map_.Levels {
		if level.Level != state.CurrentLevel {
			continue
		}

		for _, pm := range level.PlayerMap {
			if *pm.Position == position {
				return pm.LineOfSight
			}
		}
	}
	return false
}

func mapDistance(position swagger.DungeonsandtrollsPosition, state swagger.DungeonsandtrollsGameState) int {
	for _, level := range state.Map_.Levels {
		if level.Level != state.CurrentLevel {
			continue
		}

		for _, pm := range level.PlayerMap {
			if *pm.Position == position {
				return int(pm.Distance)
			}
		}
	}
	return math.MaxInt
}

func coords2pos(coords swagger.DungeonsandtrollsCoordinates) swagger.DungeonsandtrollsPosition {
	return swagger.DungeonsandtrollsPosition{
		PositionX: coords.PositionX,
		PositionY: coords.PositionY,
	}
}

func playersOnCurrentLevel(state swagger.DungeonsandtrollsGameState) []swagger.DungeonsandtrollsCharacter {
	res := []swagger.DungeonsandtrollsCharacter{}

	for _, level := range state.Map_.Levels {
		if level.Level != state.CurrentLevel {
			continue
		}

		for _, object := range level.Objects {
			res = append(res, object.Players...)
		}
	}

	return res
}
