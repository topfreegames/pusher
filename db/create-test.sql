CREATE ROLE pusher_user LOGIN
  SUPERUSER INHERIT CREATEDB CREATEROLE;

CREATE DATABASE push
  WITH OWNER = pusher_user
       ENCODING = 'UTF8'
       TABLESPACE = pg_default
       TEMPLATE = template0;

\connect push;

 CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

 CREATE TABLE "testapp_apns" (
   "id" uuid DEFAULT uuid_generate_v4(),
   "user_id" text NOT NULL,
   "token" text NOT NULL,
   "region" text NOT NULL,
   "locale" text NOT NULL,
   "tz" text NOT NULL,
   PRIMARY KEY ("id")
 );

 CREATE TABLE "testapp_gcm" (
   "id" uuid DEFAULT uuid_generate_v4(),
   "user_id" text NOT NULL,
   "token" text NOT NULL,
   "region" text NOT NULL,
   "locale" text NOT NULL,
   "tz" text NOT NULL,
   PRIMARY KEY ("id")
 );

 INSERT INTO testapp_apns (user_id, token, region, locale, tz) VALUES ('9e558649-9c23-469d-a11c-59b05813e3d5', '1234', 'BR', 'pt', '-0300');
 INSERT INTO testapp_apns (user_id, token, region, locale, tz) VALUES ('57be9009-e616-42c6-9cfe-505508ede2d0', '1235', 'US', 'en', '-0300');
 INSERT INTO testapp_apns (user_id, token, region, locale, tz) VALUES ('a8e8d2d5-f178-4d90-9b31-683ad3aae920', '1236', 'BR', 'pt', '-0300');
 INSERT INTO testapp_apns (user_id, token, region, locale, tz) VALUES ('5c3033c0-24ad-487a-a80d-68432464c8de', '1237', 'US', 'en', '-0500');
 INSERT INTO testapp_apns (user_id, token, region, locale, tz) VALUES ('4223171e-c665-4612-9edd-485f229240bf', '1238', 'BR', 'pt', '-0300');
 INSERT INTO testapp_apns (user_id, token, region, locale, tz) VALUES ('2df5bb01-15d1-4569-bc56-49fa0a33c4c3', '1239', 'US', 'en', '-0300');
 INSERT INTO testapp_apns (user_id, token, region, locale, tz) VALUES ('67b872de-8ae4-4763-aef8-7c87a7f928a7', '1244', 'BR', 'pt', '-0500');
 INSERT INTO testapp_apns (user_id, token, region, locale, tz) VALUES ('3f8732a1-8642-4f22-8d77-a9688dd6a5ae', '1245', 'BR', 'pt', '-0300');
 INSERT INTO testapp_apns (user_id, token, region, locale, tz) VALUES ('21854bbf-ea7e-43e3-8f79-9ab2c121b941', '1246', 'US', 'en', '-0300');
 INSERT INTO testapp_apns (user_id, token, region, locale, tz) VALUES ('843a61f8-45b3-44f9-9ab7-8becb2765653', '1247', 'BR', 'pt', '-0500');
 INSERT INTO testapp_apns (user_id, token, region, locale, tz) VALUES ('843a61f8-45b3-44f9-9ab7-8becb3365653', '1247', 'AU', 'au', '-0500');
 INSERT INTO testapp_apns (user_id, token, region, locale, tz) VALUES ('843a61f8-45b3-44f9-aaaa-8becb3365653', '1247', 'FR', 'fr', '-0800');
 INSERT INTO testapp_apns (user_id, token, region, locale, tz) VALUES ('843a61f8-45b3-44f9-bbbb-8becb3365653', '1247', 'FR', 'fr', '-0800');
 INSERT INTO testapp_apns (user_id, token, region, locale, tz) VALUES ('e78431ca-69a8-4326-af1f-48f817a4a669', '1247', 'ES', 'es', '-0800');
 INSERT INTO testapp_apns (user_id, token, region, locale, tz) VALUES ('d9b42bb8-78ca-44d0-ae50-a472d9fbad92', '1247', 'ES', 'es', '-0800');
 INSERT INTO testapp_apns (user_id, token, region, locale, tz) VALUES ('ee4455fe-8ff6-4878-8d7c-aec096bd68b4', '1247', 'ES', 'es', '-0800');
 INSERT INTO testapp_apns (user_id, token, region, locale, tz) VALUES ('e78431ca-69a8-4326-af1f-48f817a4a669', '1247', 'ES', 'es', '-0800');
 INSERT INTO testapp_apns (user_id, token, region, locale, tz) VALUES ('e78431ca-69a8-4326-af1f-48f817a4a669', '1248', 'ES', 'es', '-0800');

 INSERT INTO testapp_gcm (user_id, token, region, locale, tz) VALUES ('9e558649-9c23-469d-a11c-59b05000e3d5', '1234', 'br', 'PT', '-0300');
 INSERT INTO testapp_gcm (user_id, token, region, locale, tz) VALUES ('57be9009-e616-42c6-9cfe-505508ede2d0', '1235', 'us', 'EN', '-0300');
 INSERT INTO testapp_gcm (user_id, token, region, locale, tz) VALUES ('a8e8d2d5-f178-4d90-9b31-683ad3aae920', '1236', 'br', 'PT', '-0300');
 INSERT INTO testapp_gcm (user_id, token, region, locale, tz) VALUES ('5c3033c0-24ad-487a-a80d-68432464c8de', '1237', 'us', 'EN', '-0500');
 INSERT INTO testapp_gcm (user_id, token, region, locale, tz) VALUES ('4223171e-c665-4612-9edd-485f229240bf', '1238', 'br', 'PT', '-0300');
 INSERT INTO testapp_gcm (user_id, token, region, locale, tz) VALUES ('2df5bb01-15d1-4569-bc56-49fa0a33c4c3', '1239', 'us', 'EN', '-0300');
 INSERT INTO testapp_gcm (user_id, token, region, locale, tz) VALUES ('67b872de-8ae4-4763-aef8-7c87a7f928a7', '1244', 'br', 'PT', '-0500');
 INSERT INTO testapp_gcm (user_id, token, region, locale, tz) VALUES ('3f8732a1-8642-4f22-8d77-a9688dd6a5ae', '1245', 'br', 'PT', '-0300');
 INSERT INTO testapp_gcm (user_id, token, region, locale, tz) VALUES ('21854bbf-ea7e-43e3-8f79-9ab2c121b941', '1246', 'us', 'EN', '-0300');
 INSERT INTO testapp_gcm (user_id, token, region, locale, tz) VALUES ('843a61f8-45b3-44f9-9ab7-8becb2765653', '1247', 'br', 'PT', '-0500');
