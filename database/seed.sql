-- Lixr SQLite seed data.
-- Images are intentionally URL-based for this development phase.
-- The developer password is stored only as a PBKDF2-HMAC-SHA256 hash.

INSERT OR IGNORE INTO store_settings
(id,store_name,tagline,announcement,address,hours,phone,email)
VALUES
(1,'Lixr','Modern essentials. Defined by you.','NEW COLLECTION / 2026 — NOW AVAILABLE IN STORE',
'Saa, Inabanga, Bohol, Philippines','Mon–Sat 10:00–20:00 · Sun 11:00–18:00','+63 900 000 0000','hello@lixr.jpano.dev');

INSERT OR IGNORE INTO products
(slug,sku,barcode,name,subtitle,category,collection_name,description,color,size,cost_price,retail_price,sale_price,price,image_url,image_alt,badge,stock,low_stock_threshold,featured,active)
VALUES
('essential-tee','LIXR-TEE-001-BLK-M','890100000001','Lixr Essential Tee','Cotton jersey / Black','Tops','Essentials','A clean everyday tee with a relaxed silhouette and quiet Lixr detailing.','Black','M',420,1290,990,990,'https://images.unsplash.com/photo-1521572163474-6864f9cf17ab?auto=format&fit=crop&w=1200&q=88','Black essential tee','BEST SELLER',18,10,1,1),
('structured-overshirt','LIXR-OVR-002-KHK-M','890100000002','Structured Overshirt','Brushed cotton / Khaki','Outerwear','Form','A versatile overshirt designed to layer cleanly across warm and cool days.','Khaki','M',870,2490,2190,2190,'https://images.unsplash.com/photo-1598033129183-c4f50c736f10?auto=format&fit=crop&w=1200&q=88','Khaki structured overshirt','NEW',8,10,1,1),
('relaxed-trouser','LIXR-TRS-003-STN-M','890100000003','Relaxed Trouser','Drape twill / Stone','Bottoms','Essentials','Soft structure, easy movement and a neutral finish for everyday rotation.','Stone','M',760,2190,1890,1890,'https://images.unsplash.com/photo-1506629082955-511b1aa562c8?auto=format&fit=crop&w=1200&q=88','Stone relaxed trousers','LIMITED',5,10,1,1),
('fine-knit-polo','LIXR-KNT-004-OLV-M','890100000004','Fine Knit Polo','Fine knit / Olive','Knitwear','Studio','A refined knit polo with a compact collar and understated texture.','Olive','M',690,1990,1990,1990,'https://images.unsplash.com/photo-1618354691373-d851c5c3a990?auto=format&fit=crop&w=1200&q=88','Olive knit polo','',12,10,1,1),
('linen-blazer','LIXR-BLZ-005-SND-M','890100000005','Structured Linen Blazer','Linen blend / Sand','Outerwear','Quiet Form','A relaxed single-breasted blazer for polished warm-weather layering.','Sand','M',1780,4290,4290,4290,'https://images.unsplash.com/photo-1591047139829-d91aecb6caea?auto=format&fit=crop&w=1200&q=88','Sand tailored blazer','NEW',7,10,1,1),
('rib-tank','LIXR-TNK-006-BNE-S','890100000006','Essential Rib Tank','Cotton rib / Bone','Tops','Core','A close-cut ribbed tank with a soft handfeel for layering or solo wear.','Bone','S',440,1290,1190,1190,'https://images.unsplash.com/photo-1521572163474-6864f9cf17ab?auto=format&fit=crop&w=1200&q=88','Neutral rib tank','',22,10,0,1),
('wide-leg-trouser','LIXR-TRS-007-CHR-M','890100000007','Fluid Wide-Leg Trouser','Drape twill / Charcoal','Bottoms','Quiet Form','High-rise trousers with fluid movement and a clean tailored break.','Charcoal','M',980,3190,2790,2790,'https://images.unsplash.com/photo-1594633312681-425c7b97ccd1?auto=format&fit=crop&w=1200&q=88','Charcoal wide leg trousers','',3,10,0,1),
('column-midi-dress','LIXR-DRS-008-BLK-S','890100000008','Column Midi Dress','Stretch crepe / Black','Dresses','After Dark','A minimal column dress with a sculpted neckline and fluid finish.','Black','S',1420,3590,3290,3290,'https://images.unsplash.com/photo-1566174053879-31528523f8ae?auto=format&fit=crop&w=1200&q=88','Black column midi dress','NEW',6,10,0,1),
('everyday-overshirt','LIXR-OVR-009-OLV-L','890100000009','Everyday Overshirt','Brushed cotton / Olive','Outerwear','Core','An easy overshirt with practical pockets and a structured comfortable shape.','Olive','L',1040,2690,2490,2490,'https://images.unsplash.com/photo-1603252110481-7ba873bf42ab?auto=format&fit=crop&w=1200&q=88','Olive everyday overshirt','',9,10,0,1),
('studio-shirt','LIXR-SHT-010-WHT-M','890100000010','Studio Shirt','Poplin / White','Shirts','Studio','A crisp oversized shirt with restrained proportions and a clean collar.','White','M',620,1790,1590,1590,'https://images.unsplash.com/photo-1602810318383-e386cc2a3ccf?auto=format&fit=crop&w=1200&q=88','White studio shirt','',14,10,0,1),
('tailored-vest','LIXR-VST-011-CRM-S','890100000011','Tailored Vest','Suiting / Cream','Tops','Quiet Form','A fitted sleeveless tailoring layer designed for clean tonal styling.','Cream','S',830,2290,2190,2190,'https://images.unsplash.com/photo-1591369822096-ffd140ec948f?auto=format&fit=crop&w=1200&q=88','Cream tailored vest','LIMITED',4,10,0,1),
('mini-shoulder-bag','LIXR-BAG-012-BRN-OS','890100000012','Mini Shoulder Bag','Vegan leather / Espresso','Accessories','Objects','A compact shoulder bag with minimal hardware and structured geometry.','Espresso','One size',920,2490,2290,2290,'https://images.unsplash.com/photo-1584917865442-de89df76afd3?auto=format&fit=crop&w=1200&q=88','Espresso shoulder bag','',11,10,0,1);

-- Customer reviews are created by signed-in visitors.

INSERT OR IGNORE INTO staff_accounts
(username,email,display_name,role,password_salt,password_hash,password_iterations,active,protected)
VALUES
('toneiu',NULL,'Toneiu Developer','developer','TElYUkRFVkVMT1BFUjIwMg','y8VfVbdLuTuZVE0QKHA3ztEnYPfVt2S7Q84QeJVNG5s',180000,1,1);
