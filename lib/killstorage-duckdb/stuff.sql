attach 'host=127.0.0.1 dbname=thunder user=thunder password=warthunder_analytics_or_something' as pgdb (type postgres, schema 'public', read_only);

.timer on

create table kills as select
	session,
	session_time,
	kill_time,
	ln.name as level,
	mn.name as mission,
	killer_id,
	killer_team,
	kvn.name as killer_vehicle,
	killer_posx,
	killer_posz,
	vn.name as weapon,
	victim_id,
	victim_team,
	vvn.name as victim_vehicle,
	victim_posx,
	victim_posz
from pgdb.kills as k
left join pgdb.level_names as ln on k.level = ln.id
left join pgdb.mission_names as mn on k.mission = mn.id
left join pgdb.weapon_names as vn on k.weapon = vn.id
left join pgdb.vehicle_names as kvn on k.killer_vehicle = kvn.id
left join pgdb.vehicle_names as vvn on k.victim_vehicle = vvn.id;

alter table kills alter column session set not null;
alter table kills alter column session_time set not null;
alter table kills alter column kill_time set not null;
alter table kills alter column level set not null;
alter table kills alter column mission set not null;
alter table kills alter column killer_id set not null;
alter table kills alter column killer_team set not null;
alter table kills alter column killer_vehicle set not null;
alter table kills alter column killer_posx set not null;
alter table kills alter column killer_posz set not null;
alter table kills alter column weapon set not null;
alter table kills alter column victim_id set not null;
alter table kills alter column victim_team set not null;
alter table kills alter column victim_vehicle set not null;
alter table kills alter column victim_posx set not null;
alter table kills alter column victim_posz set not null;

select
	vn.name, sum(p.seen) as s
from kills k
cross join lateral (
	values
		(k.killer_vehicle, 1),
		(k.victim_vehicle, 1)
) as p(vehicle, seen)
left join vehicle_names as vn on vn.id = p.vehicle
group by vn.name
order by s desc
