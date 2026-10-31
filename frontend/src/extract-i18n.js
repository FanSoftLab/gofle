#!/usr/bin/env node
/**
 * Extract translations from i18n.ts and create JSON files
 */

const fs = require('fs');
const path = require('path');

const I18N_TS_PATH = path.join(__dirname, 'i18n.ts');
const I18N_DIR = path.join(__dirname, 'i18n');
const UK_JSON_PATH = path.join(I18N_DIR, 'uk.json');
const EN_JSON_PATH = path.join(I18N_DIR, 'en.json');

function parseTranslations(content) {
    // Find the translations object
    const match = content.match(/export const translations:\s*Record<LangCode,\s*Record<string,\s*string>>\s*=\s*(\{[\s\S]*?\n\});/);
    if (!match) {
        throw new Error('Could not find translations object in i18n.ts');
    }

    const translationsStr = match[1];
    
    // Parse the TypeScript object into a JavaScript object
    // We'll use a simple approach: replace TypeScript-specific syntax and eval
    let jsCode = translationsStr
        // Remove trailing commas before closing braces/brackets
        .replace(/,(\s*[}\]])/g, '$1')
        // Handle the Record type annotation (already removed by regex)
        ;
    
    // Use Function constructor to safely parse the object
    const translations = new Function('return ' + jsCode)();
    
    return translations;
}

function sortKeys(obj) {
    const sorted = {};
    Object.keys(obj).sort().forEach(key => {
        sorted[key] = obj[key];
    });
    return sorted;
}

function main() {
    console.log('Reading i18n.ts...');
    const content = fs.readFileSync(I18N_TS_PATH, 'utf-8');
    
    console.log('Parsing translations...');
    const translations = parseTranslations(content);
    
    if (!translations.uk || !translations.en) {
        throw new Error('Missing uk or en translations');
    }
    
    console.log(`Found ${Object.keys(translations.uk).length} Ukrainian translations`);
    console.log(`Found ${Object.keys(translations.en).length} English translations`);
    
    // Sort keys alphabetically
    const ukSorted = sortKeys(translations.uk);
    const enSorted = sortKeys(translations.en);
    
    // Create i18n directory if it doesn't exist
    if (!fs.existsSync(I18N_DIR)) {
        console.log('Creating i18n directory...');
        fs.mkdirSync(I18N_DIR, { recursive: true });
    }
    
    // Write JSON files with 2-space indentation
    console.log('Writing uk.json...');
    fs.writeFileSync(UK_JSON_PATH, JSON.stringify(ukSorted, null, 2) + '\n', 'utf-8');
    
    console.log('Writing en.json...');
    fs.writeFileSync(EN_JSON_PATH, JSON.stringify(enSorted, null, 2) + '\n', 'utf-8');
    
    // Show results
    console.log('\n✓ Done!');
    console.log(`  ${UK_JSON_PATH}`);
    console.log(`  ${EN_JSON_PATH}`);
    
    // Show a preview
    console.log('\nPreview (first 5 keys each):');
    console.log('\nUkrainian:');
    Object.entries(ukSorted).slice(0, 5).forEach(([k, v]) => console.log(`  "${k}": "${v}"`));
    console.log('  ...');
    
    console.log('\nEnglish:');
    Object.entries(enSorted).slice(0, 5).forEach(([k, v]) => console.log(`  "${k}": "${v}"`));
    console.log('  ...');
}

main().catch(err => {
    console.error('Error:', err.message);
    process.exit(1);
});