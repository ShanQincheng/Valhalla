package game

import (
	"github.com/Hucaru/Valhalla/game/def"
	"github.com/Hucaru/Valhalla/game/packet"
	"github.com/Hucaru/Valhalla/mnet"
	"github.com/Hucaru/Valhalla/nx"
)

var maps = make(map[int32]*GameMap)

type GameMap struct {
	npcs []def.NPC
	mobs []gameMob
	id   int32
}

func InitMaps() {
	for mapID, nxMap := range nx.GetMaps() {
		npcs := []def.NPC{}
		mobs := []gameMob{}

		for _, l := range nxMap.Mobs {
			nxMob, err := nx.GetMob(l.ID)

			if err != nil {
				continue
			}

			mobs = append(mobs, gameMob{Mob: def.CreateMob(int32(len(mobs)+1), l, nxMob, nil), mapID: mapID})
		}

		for _, l := range nxMap.NPCs {
			npcs = append(npcs, def.CreateNPC(int32(len(npcs)), l))
		}

		maps[mapID] = &GameMap{
			npcs: npcs,
			mobs: mobs,
			id:   mapID,
		}
	}
}

func (gm *GameMap) removeController(conn mnet.MConnChannel) {
	for i, m := range gm.mobs {
		if m.Controller == conn {
			gm.mobs[i].Controller = nil
			conn.Send(packet.MobEndControl(m.Mob))
		}
	}

	for c, p := range players {
		if c != conn && p.char.MapID == players[conn].char.MapID {
			for i, m := range gm.mobs {
				gm.mobs[i].Controller = c
				c.Send(packet.MobControl(m.Mob))
			}
		}
	}
}

func (gm *GameMap) addController(conn mnet.MConnChannel) {
	for i, m := range gm.mobs {
		if m.Controller == nil {
			gm.mobs[i].Controller = conn
			conn.Send(packet.MobControl(m.Mob))
		}
	}
}

func (gm *GameMap) GetMobFromID(id int32) *gameMob {
	for i, v := range gm.mobs {
		if v.SpawnID == id {
			return &gm.mobs[i]
		}
	}

	return nil
}

func (gm *GameMap) GetNPCFromID(id int32) *def.NPC {
	for i, v := range gm.npcs {
		if v.SpawnID == id {
			return &gm.npcs[i]
		}
	}

	return nil
}

func (gm GameMap) generateMobSpawnID() int32 {
	var l int32
	for _, v := range gm.mobs {
		if v.SpawnID > l {
			l = v.SpawnID
		}
	}

	l++

	if l == 0 {
		l++
	}

	return l
}

func (gm *GameMap) HandleDeadMobs() {
	y := gm.mobs[:0]

	for _, mob := range gm.mobs {
		if mob.HP < 1 {
			mob.Controller.Send(packet.MobEndControl(mob.Mob))

			for _, id := range mob.Revives {
				gm.SpawnMobNoRespawn(id, gm.generateMobSpawnID(), mob.X, mob.Y, mob.Foothold, -3, mob.SpawnID, mob.FacesLeft())
				y = append(y, gm.mobs[len(gm.mobs)-1])
			}

			SendToMap(mob.mapID, packet.MobRemove(mob.Mob, 1)) // 0 keeps it there and is no longer attackable, 1 normal death, 2 disaapear instantly
		} else {
			y = append(y, mob)
		}
	}

	gm.mobs = y
}

func (gm *GameMap) SpawnMob(mobID, spawnID int32, x, y, foothold int16, summonType int8, summonOption int32, facesLeft bool) {

}

func (gm *GameMap) SpawnMobNoRespawn(mobID, spawnID int32, x, y, foothold int16, summonType int8, summonOption int32, facesLeft bool) {
	m, err := nx.GetMob(mobID)

	if err != nil {
		return
	}

	mob := def.CreateMob(spawnID, nx.Life{}, m, nil)
	mob.ID = mobID

	mob.X = x
	mob.Y = y
	mob.Foothold = foothold

	mob.Respawns = false

	mob.SummonType = summonType
	mob.SummonOption = summonOption

	mob.FaceLeft = facesLeft

	SendToMap(gm.id, packet.MobShow(mob))

	if summonType != -4 {
		mob.SummonType = -1
		mob.SummonOption = 0
	}

	gm.mobs = append(gm.mobs, gameMob{Mob: mob, mapID: gm.id})

	findController(gm.id, &gm.mobs[len(gm.mobs)-1])
}

func findController(mapID int32, mob *gameMob) {
	for _, p := range players {
		if p.char.MapID == mapID {
			mob.ChangeController(p)
			return
		}
	}
}
