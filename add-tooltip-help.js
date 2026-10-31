const fs = require('fs');
const path = require('path');

const filePath = path.join(__dirname, 'frontend/src/i18n.ts');
const content = fs.readFileSync(filePath, 'utf8');

// Find the tooltip_settings line in the en object and add tooltip_help after it
const searchPattern = /(tooltip_settings: "Settings",)/;
const replacement = `$1\n        tooltip_help: "Help (F1)",`;

const newContent = content.replace(searchPattern, replacement);

if (newContent === content) {
    console.log('No changes made - pattern not found or already exists');
} else {
    fs.writeFileSync(filePath, newContent, 'utf8');
    console.log('Successfully added tooltip_help to en object');
}