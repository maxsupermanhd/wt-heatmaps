attach 'host=127.0.0.1 dbname=thunder user=thunder password=warthunder_analytics_or_something' as pgdb (type postgres, schema 'public', read_only);

.timer on

create table if not exists kills as from pgdb.kills;
create table if not exists level_names as from pgdb.level_names;
create table if not exists mission_names as from pgdb.mission_names;
create table if not exists vehicle_names as from pgdb.vehicle_names;
create table if not exists weapon_names as from pgdb.weapon_names;

select max(id) from level_names;
select max(id) from mission_names;
select max(id) from vehicle_names;
select max(id) from weapon_names;

create sequence if not exists level_names_id_seq start 55;
create sequence if not exists mission_names_id_seq start 301;
create sequence if not exists vehicle_names_id_seq start 2690;
create sequence if not exists weapon_names_id_seq start 1712;


alter table level_names rename column old_name to name;
alter table mission_names rename column old_name to name;
alter table vehicle_names rename column old_name to name;
alter table weapon_names rename column old_name to name;


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
